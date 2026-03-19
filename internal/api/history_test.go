// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"testing"
	"time"
)

func TestHistoryBuffer_OrderAndEviction(t *testing.T) {
	h := NewHistoryBuffer(3)
	t0 := time.Unix(100, 0).UTC()
	t1 := time.Unix(200, 0).UTC()
	t2 := time.Unix(300, 0).UTC()
	t3 := time.Unix(400, 0).UTC()

	h.Add(TelemetryPoint{Timestamp: t0, TemperatureC: 1})
	h.Add(TelemetryPoint{Timestamp: t1, TemperatureC: 2})
	h.Add(TelemetryPoint{Timestamp: t2, TemperatureC: 3})
	h.Add(TelemetryPoint{Timestamp: t3, TemperatureC: 4})

	since := time.Unix(0, 0).UTC()
	got := h.Points(since)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0].TemperatureC != 2 || got[1].TemperatureC != 3 || got[2].TemperatureC != 4 {
		t.Fatalf("order wrong: %+v", got)
	}
}

func TestHistoryBuffer_PointsSince(t *testing.T) {
	h := NewHistoryBuffer(10)
	t100 := time.Unix(100, 0).UTC()
	t200 := time.Unix(200, 0).UTC()
	t300 := time.Unix(300, 0).UTC()
	h.Add(TelemetryPoint{Timestamp: t100, TemperatureC: 1})
	h.Add(TelemetryPoint{Timestamp: t200, TemperatureC: 2})
	h.Add(TelemetryPoint{Timestamp: t300, TemperatureC: 3})

	got := h.Points(time.Unix(150, 0).UTC())
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].TemperatureC != 2 || got[1].TemperatureC != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestHistoryBuffer_Empty(t *testing.T) {
	h := NewHistoryBuffer(5)
	got := h.Points(time.Time{})
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", got)
	}
}

func TestHistoryBuffer_MaxZero(t *testing.T) {
	h := NewHistoryBuffer(0)
	h.Add(TelemetryPoint{Timestamp: time.Now().UTC(), TemperatureC: 1})
	got := h.Points(time.Time{})
	if len(got) != 0 {
		t.Fatalf("want no points")
	}
}
