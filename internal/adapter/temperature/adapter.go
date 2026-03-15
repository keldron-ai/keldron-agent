package temperature

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/health"
)

const channelBuffer = 256

// pollSNMPFunc and pollModbusFunc are used for polling; tests may replace them with mocks.
var pollSNMPFunc = PollSNMP
var pollModbusFunc = PollModbus

// TemperatureAdapter polls inlet/outlet temperatures via SNMP or Modbus TCP.
type TemperatureAdapter struct {
	cfg          config.AdapterConfig
	tempCfg      TemperatureConfig
	stale        *StaleDetector
	staleThresh  int
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	pollInterval time.Duration
	intervalMu   sync.RWMutex
	intervalChMu sync.Mutex    // protects intervalCh close-and-replace
	intervalCh   chan struct{} // closed to broadcast interval changes
	unsubOnce    sync.Once     // ensures unsubscribe runs exactly once
	unsubscribe  func()        // removes config subscription
	closeOnce    sync.Once
	startedOnce  atomic.Bool // true after first Start; prevents reuse after shutdown

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates a TemperatureAdapter from the adapter config.
// This is the Constructor registered in the adapter registry.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var tempCfg TemperatureConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&tempCfg); err != nil {
			return nil, fmt.Errorf("decoding temperature config: %w", err)
		}
	}

	if len(tempCfg.Sensors) == 0 {
		return nil, fmt.Errorf("temperature adapter requires at least one sensor")
	}

	seenIDs := make(map[string]bool, len(tempCfg.Sensors))
	for i, s := range tempCfg.Sensors {
		if s.SensorID == "" {
			return nil, fmt.Errorf("sensor at index %d has empty sensor_id", i)
		}
		if seenIDs[s.SensorID] {
			return nil, fmt.Errorf("duplicate sensor_id %q at index %d", s.SensorID, i)
		}
		seenIDs[s.SensorID] = true
		switch s.Protocol {
		case "snmp", "modbus":
			// valid
		default:
			return nil, fmt.Errorf("sensor %q has invalid protocol %q (must be snmp or modbus)", s.SensorID, s.Protocol)
		}
		switch s.Position {
		case "inlet", "outlet", "":
			// valid
		default:
			return nil, fmt.Errorf("sensor %q has invalid position %q (must be inlet or outlet)", s.SensorID, s.Position)
		}
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	staleThreshold := tempCfg.StaleThreshold
	if staleThreshold <= 0 {
		staleThreshold = 5
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &TemperatureAdapter{
		cfg:          cfg,
		tempCfg:      tempCfg,
		stale:        NewStaleDetector(staleThreshold),
		staleThresh:  staleThreshold,
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       logger,
		holder:       holder,
		pollInterval: interval,
		intervalCh:   make(chan struct{}),
	}, nil
}

// Name returns the adapter identifier.
func (t *TemperatureAdapter) Name() string { return "temperature" }

// Readings returns the channel of raw readings.
func (t *TemperatureAdapter) Readings() <-chan adapter.RawReading { return t.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (t *TemperatureAdapter) IsRunning() bool {
	return t.running.Load()
}

// Stats returns poll count, error count, last poll time, last error, and last error time for health reporting.
func (t *TemperatureAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = t.pollCount.Load()
	errorCount = t.errorCount.Load()
	if v := t.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	if v := t.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := t.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, lastError, lastErrorAt
}

// Start begins polling all sensors. Spawns one goroutine per sensor.
// Blocks until ctx is cancelled.
func (t *TemperatureAdapter) Start(ctx context.Context) error {
	if t.startedOnce.Load() {
		return fmt.Errorf("temperature adapter cannot be restarted after shutdown")
	}
	if !t.running.CompareAndSwap(false, true) {
		return fmt.Errorf("temperature adapter already started")
	}
	t.startedOnce.Store(true)
	defer t.running.Store(false)

	t.logger.Info("temperature adapter polling started",
		"interval", t.pollInterval,
		"sensors", len(t.tempCfg.Sensors),
		"stale_threshold", t.staleThresh,
	)

	if t.holder != nil {
		t.unsubscribe = t.holder.Subscribe(func(cfg *config.Config) {
			acfg, ok := cfg.Adapters["temperature"]
			if !ok {
				return
			}
			t.updatePollInterval(acfg.PollInterval)
		})
	}

	var wg sync.WaitGroup
	for i := range t.tempCfg.Sensors {
		sensor := t.tempCfg.Sensors[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			t.pollSensor(ctx, sensor)
		}()
	}

	// Wait for context cancellation; sensors stop when ctx is done
	<-ctx.Done()

	t.logger.Info("temperature adapter stopping")
	wg.Wait()
	t.doUnsubscribe()
	t.closeOnce.Do(func() { close(t.readings) })

	return nil
}

func (t *TemperatureAdapter) doUnsubscribe() {
	t.unsubOnce.Do(func() {
		if t.unsubscribe != nil {
			t.unsubscribe()
		}
	})
}

func (t *TemperatureAdapter) getPollInterval() time.Duration {
	t.intervalMu.RLock()
	defer t.intervalMu.RUnlock()
	return t.pollInterval
}

func (t *TemperatureAdapter) updatePollInterval(newInterval time.Duration) {
	if newInterval <= 0 {
		newInterval = 30 * time.Second
	}
	t.intervalMu.Lock()
	old := t.pollInterval
	if newInterval == old {
		t.intervalMu.Unlock()
		return
	}
	t.pollInterval = newInterval
	t.intervalMu.Unlock()
	t.logger.Info("poll interval updated", "old", old, "new", newInterval)
	// Broadcast to all pollSensor goroutines by closing and replacing the channel
	t.intervalChMu.Lock()
	close(t.intervalCh)
	t.intervalCh = make(chan struct{})
	t.intervalChMu.Unlock()
}

// Stop gracefully shuts down the adapter.
func (t *TemperatureAdapter) Stop(_ context.Context) error {
	t.logger.Info("temperature adapter shutting down")
	t.doUnsubscribe()
	return nil
}

func (t *TemperatureAdapter) getIntervalCh() chan struct{} {
	t.intervalChMu.Lock()
	defer t.intervalChMu.Unlock()
	return t.intervalCh
}

// pollSensor polls a single sensor on an interval until ctx is cancelled.
func (t *TemperatureAdapter) pollSensor(ctx context.Context, sensor SensorConfig) {
	ticker := time.NewTicker(t.getPollInterval())
	defer ticker.Stop()

	intervalCh := t.getIntervalCh()

	// Initial poll immediately
	t.pollOne(sensor)

	for {
		select {
		case <-ctx.Done():
			return
		case <-intervalCh:
			intervalCh = t.getIntervalCh()
			ticker.Reset(t.getPollInterval())
		case <-ticker.C:
			t.pollOne(sensor)
			// If updatePollInterval closed and replaced intervalCh while
			// pollOne was blocked, the local intervalCh is stale.
			// Re-read via getIntervalCh and reset the ticker via
			// getPollInterval so the next tick uses the updated interval.
			if newCh := t.getIntervalCh(); newCh != intervalCh {
				intervalCh = newCh
				ticker.Reset(t.getPollInterval())
			}
		}
	}
}

// pollOne performs one poll of a sensor and sends the reading if successful.
func (t *TemperatureAdapter) pollOne(sensor SensorConfig) {
	var reading adapter.RawReading
	var err error

	switch sensor.Protocol {
	case "snmp":
		reading, err = pollSNMPFunc(sensor)
	case "modbus":
		reading, err = pollModbusFunc(sensor)
	default:
		err = fmt.Errorf("unknown protocol %q for sensor %s", sensor.Protocol, sensor.SensorID)
	}

	if err != nil {
		t.errorCount.Add(1)
		t.lastError.Store(err.Error())
		t.lastErrorAt.Store(time.Now())
		t.logger.Warn("temperature poll failed",
			"sensor_id", sensor.SensorID,
			"protocol", sensor.Protocol,
			"error", err,
		)
		return
	}

	t.pollCount.Add(1)
	t.lastPoll.Store(time.Now())

	// Extract temp value for stale detection
	tempC, hasTemp := extractTempFromReading(reading)
	var isStale bool
	if hasTemp {
		isStale = t.stale.Check(sensor.SensorID, tempC)
	}

	if isStale {
		t.logger.Warn("temperature sensor unchanged for multiple intervals",
			"sensor_id", sensor.SensorID,
			"intervals", t.staleThresh,
		)
		reading.Metrics["stale"] = 1.0
	} else {
		reading.Metrics["stale"] = 0.0
	}

	select {
	case t.readings <- reading:
	default:
		t.logger.Warn("readings channel full, dropping",
			"sensor_id", sensor.SensorID,
		)
	}
}

// extractTempFromReading returns the temperature value and true if found,
// or (0, false) if no valid temperature metric is present.
func extractTempFromReading(r adapter.RawReading) (float64, bool) {
	if v, ok := r.Metrics["inlet_temp_c"]; ok {
		if f, ok := toFloat64(v); ok {
			return f, true
		}
	}
	if v, ok := r.Metrics["outlet_temp_c"]; ok {
		if f, ok := toFloat64(v); ok {
			return f, true
		}
	}
	return 0, false
}

func toFloat64(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

// Ensure TemperatureAdapter implements health.AdapterProvider.
var _ health.AdapterProvider = (*TemperatureAdapter)(nil)
