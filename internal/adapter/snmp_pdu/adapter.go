package snmp_pdu

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
)

const (
	channelBuffer = 256
)

// SNMPPDUAdapter implements the Adapter interface for PDU power monitoring via SNMP.
type SNMPPDUAdapter struct {
	cfg          config.AdapterConfig
	snmpCfg      *SNMPPDUConfig
	pollers      []*SNMPPoller
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	mu           sync.Mutex
	closeOnce    sync.Once
	pollInterval time.Duration
	ticker       *time.Ticker

	unsubscribeConfig func()
	cancel            context.CancelFunc
	stopped           bool // guarded by mu; set in Stop to prevent late reload swaps

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates an SNMPPDUAdapter from the adapter config.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	if logger == nil {
		logger = slog.Default()
	}
	snmpCfg, err := DecodeFromRaw(&cfg.Raw)
	if err != nil {
		return nil, fmt.Errorf("snmp_pdu config: %w", err)
	}
	snmpCfg.ApplyDefaults()
	if err := snmpCfg.Validate(); err != nil {
		return nil, err
	}

	pollers, err := createPollers(snmpCfg, logger.With("adapter", adapterName))
	if err != nil {
		return nil, fmt.Errorf("creating pollers: %w", err)
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	return &SNMPPDUAdapter{
		cfg:          cfg,
		snmpCfg:      snmpCfg,
		pollers:      pollers,
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       logger,
		holder:       holder,
		pollInterval: interval,
	}, nil
}

func createPollers(cfg *SNMPPDUConfig, logger *slog.Logger) ([]*SNMPPoller, error) {
	var pollers []*SNMPPoller
	for i := range cfg.Targets {
		p, err := NewSNMPPoller(cfg.Targets[i], cfg, logger)
		if err != nil {
			// Close any already-created pollers
			for _, existing := range pollers {
				_ = existing.Close()
			}
			return nil, fmt.Errorf("target %s: %w", cfg.Targets[i].PDUID, err)
		}
		pollers = append(pollers, p)
	}
	return pollers, nil
}

// Name returns the adapter identifier.
func (a *SNMPPDUAdapter) Name() string { return adapterName }

// Readings returns the channel of raw readings.
func (a *SNMPPDUAdapter) Readings() <-chan adapter.RawReading { return a.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (a *SNMPPDUAdapter) IsRunning() bool {
	return a.running.Load()
}

// Stats returns poll count, error count, last poll time, last error for health reporting.
func (a *SNMPPDUAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = a.pollCount.Load()
	errorCount = a.errorCount.Load()
	if v := a.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	if v := a.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := a.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, lastError, lastErrorAt
}

// Start begins the polling loop. Blocks until ctx is cancelled or Stop is called.
func (a *SNMPPDUAdapter) Start(ctx context.Context) error {
	a.running.Store(true)
	defer a.running.Store(false)

	ctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.ticker = time.NewTicker(a.pollInterval)
	a.mu.Unlock()

	if a.holder != nil {
		unsub := a.holder.Subscribe(func(cfg *config.Config) {
			a.handleConfigChange(cfg)
		})
		a.mu.Lock()
		a.unsubscribeConfig = unsub
		a.mu.Unlock()
	}

	a.logger.Info("SNMP PDU adapter polling started",
		"interval", a.pollInterval,
		"targets", len(a.pollers),
	)

	// Initial poll immediately.
	a.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("SNMP PDU adapter stopping")
			a.mu.Lock()
			if a.ticker != nil {
				a.ticker.Stop()
			}
			a.mu.Unlock()
			a.closePollers()
			a.closeOnce.Do(func() {
				close(a.readings)
			})
			return nil
		case <-a.ticker.C:
			a.poll(ctx)
		}
	}
}

func (a *SNMPPDUAdapter) handleConfigChange(cfg *config.Config) {
	acfg, ok := cfg.Adapters[adapterName]
	if !ok {
		return
	}

	// Update poll interval
	newInterval := acfg.PollInterval
	if newInterval <= 0 {
		newInterval = 30 * time.Second
	}
	a.mu.Lock()
	if newInterval != a.pollInterval {
		a.logger.Info("poll interval updated", "old", a.pollInterval, "new", newInterval)
		a.pollInterval = newInterval
		if a.ticker != nil {
			a.ticker.Reset(newInterval)
		}
	}
	a.mu.Unlock()

	// Decode new snmp_pdu config and check if targets changed
	snmpCfg, err := DecodeFromRaw(&acfg.Raw)
	if err != nil {
		a.logger.Warn("config reload: failed to decode snmp_pdu config", "error", err)
		return
	}
	snmpCfg.ApplyDefaults()
	if err := snmpCfg.Validate(); err != nil {
		a.logger.Warn("config reload: invalid snmp_pdu config", "error", err)
		return
	}

	if !targetsEqual(a.snmpCfg.Targets, snmpCfg.Targets) {
		a.logger.Info("config reload: targets changed, recreating pollers")
		newPollers, err := createPollers(snmpCfg, a.logger)
		if err != nil {
			a.logger.Error("config reload: failed to create pollers", "error", err)
			a.errorCount.Add(1)
			a.lastError.Store(err.Error())
			a.lastErrorAt.Store(time.Now())
			return
		}
		a.mu.Lock()
		if a.stopped {
			a.mu.Unlock()
			// Adapter is shutting down — discard the new pollers.
			for _, p := range newPollers {
				_ = p.Close()
			}
			return
		}
		oldPollers := a.pollers
		a.pollers = newPollers
		a.snmpCfg = snmpCfg
		a.mu.Unlock()
		for _, p := range oldPollers {
			_ = p.Close()
		}
	}
}

func targetsEqual(a, b []PDUTarget) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(t PDUTarget) string { return t.Address + "|" + t.PDUID }
	am := make(map[string][]PDUTarget)
	for _, t := range a {
		k := key(t)
		am[k] = append(am[k], t)
	}
	for _, t := range b {
		k := key(t)
		entries := am[k]
		idx := -1
		for i, e := range entries {
			if e.Vendor == t.Vendor && rackIDsEqual(e.RackIDs, t.RackIDs) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return false
		}
		// Remove matched entry to handle duplicates correctly.
		am[k] = append(entries[:idx], entries[idx+1:]...)
	}
	return true
}

func rackIDsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, r := range a {
		set[r]++
	}
	for _, r := range b {
		set[r]--
		if set[r] < 0 {
			return false
		}
	}
	return true
}

// closePollers detaches the poller slice under a.mu, then closes each poller
// outside the lock so Close() cannot deadlock with concurrent poll() calls.
func (a *SNMPPDUAdapter) closePollers() {
	a.mu.Lock()
	tmp := a.pollers
	a.pollers = nil
	a.mu.Unlock()
	for _, p := range tmp {
		_ = p.Close()
	}
}

// Stop gracefully shuts down the adapter.
func (a *SNMPPDUAdapter) Stop(_ context.Context) error {
	a.logger.Info("SNMP PDU adapter shutting down")
	a.mu.Lock()
	a.stopped = true
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if a.unsubscribeConfig != nil {
		a.unsubscribeConfig()
		a.unsubscribeConfig = nil
	}
	a.mu.Unlock()
	a.closePollers()
	return nil
}

func (a *SNMPPDUAdapter) poll(ctx context.Context) {
	a.mu.Lock()
	pollers := append([]*SNMPPoller(nil), a.pollers...)
	a.mu.Unlock()

	if len(pollers) == 0 {
		return
	}

	var wg sync.WaitGroup
	results := make(chan []adapter.RawReading, len(pollers))
	errCh := make(chan error, len(pollers))

	for _, p := range pollers {
		wg.Add(1)
		go func(poller *SNMPPoller) {
			defer wg.Done()
			readings, err := poller.Poll(ctx)
			if err != nil {
				errCh <- err
				return
			}
			if len(readings) > 0 {
				results <- readings
			}
		}(p)
	}

	go func() {
		wg.Wait()
		close(results)
		close(errCh)
	}()

	var errCount int
	for err := range errCh {
		errCount++
		a.logger.Error("PDU poll failed", "error", err)
	}

	for readings := range results {
		for _, r := range readings {
			select {
			case a.readings <- r:
			default:
				a.logger.Warn("readings channel full, dropping reading", "source", r.Source)
			}
		}
	}

	// Stats semantics: pollCount increments only when all pollers succeed
	// (errCount == 0). errorCount accumulates individual poller failures.
	// Partial-success cycles increment only errorCount/lastError/lastErrorAt,
	// not pollCount. Health consumers should treat a rising errorCount with
	// stale lastPoll as degraded.
	if errCount > 0 {
		a.errorCount.Add(uint64(errCount))
		a.lastError.Store("one or more PDUs failed")
		a.lastErrorAt.Store(time.Now())
	} else {
		a.pollCount.Add(1)
		a.lastPoll.Store(time.Now())
	}
}
