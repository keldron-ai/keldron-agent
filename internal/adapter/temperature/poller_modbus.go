// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package temperature

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/grid-x/modbus"
	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

// PollModbus reads a holding or input register via Modbus TCP and returns a RawReading.
// Applies scale_factor to convert raw register value to °C.
func PollModbus(cfg SensorConfig) (adapter.RawReading, error) {
	switch cfg.RegisterType {
	case "input", "holding", "":
		// valid
	default:
		return adapter.RawReading{}, fmt.Errorf("invalid register_type %q (use holding or input)", cfg.RegisterType)
	}

	handler := modbus.NewTCPClientHandler(cfg.Address)
	handler.SetSlave(cfg.UnitID)
	handler.Timeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := handler.Connect(ctx); err != nil {
		return adapter.RawReading{}, fmt.Errorf("connect: %w", err)
	}
	defer handler.Close()

	client := modbus.NewClient(handler)

	var results []byte
	var err error
	switch cfg.RegisterType {
	case "input":
		results, err = client.ReadInputRegisters(ctx, cfg.RegisterAddress, 1)
	default: // "holding" or ""
		results, err = client.ReadHoldingRegisters(ctx, cfg.RegisterAddress, 1)
	}
	if err != nil {
		return adapter.RawReading{}, fmt.Errorf("read registers: %w", err)
	}
	if len(results) < 2 {
		return adapter.RawReading{}, fmt.Errorf("insufficient data: got %d bytes", len(results))
	}

	raw := binary.BigEndian.Uint16(results[:2])
	scale := cfg.ScaleFactor
	if scale == 0 {
		scale = 0.1
	}
	tempC := float64(raw) * scale

	metricKey := metricKeyForPosition(cfg.Position)
	metrics := map[string]interface{}{
		metricKey: tempC,
		"stale":   0.0,
	}

	return adapter.RawReading{
		AdapterName: "temperature",
		Source:      cfg.SensorID,
		Timestamp:   time.Now(),
		Metrics:     metrics,
	}, nil
}
