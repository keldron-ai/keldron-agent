package buffer

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(
		devNull{}, &slog.HandlerOptions{Level: slog.LevelError + 1},
	))
}

type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

func walTestPoint(id string) normalizer.TelemetryPoint {
	return normalizer.TelemetryPoint{
		ID:          id,
		AgentID:     "agent-test",
		AdapterName: "test",
		Source:      "src",
		RackID:      "rack",
		Timestamp:   time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC),
		ReceivedAt:  time.Date(2025, 7, 1, 12, 0, 1, 0, time.UTC),
		Metrics:     map[string]float64{"v": 1.0},
	}
}

func TestWAL_SegmentNumericSortOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create segment files with non-zero-padded names that would mis-sort lexicographically.
	// Lexicographic: 1, 10, 11, 12, 2, 3, 4, 5, 6, 7, 8, 9
	// Numeric:       1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12
	ids := []string{"p01", "p02", "p03", "p04", "p05", "p06", "p07", "p08", "p09", "p10", "p11", "p12"}
	for i, id := range ids {
		seq := i + 1
		name := fmt.Sprintf("wal-%d.seg", seq) // non-zero-padded
		path := filepath.Join(dir, name)

		data, err := encodeRecord(walTestPoint(id))
		if err != nil {
			t.Fatalf("encodeRecord(%s): %v", id, err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ch := w.Drain(context.Background())
	var got []string
	for p := range ch {
		got = append(got, p.ID)
	}

	if len(got) != len(ids) {
		t.Fatalf("drained %d points, want %d", len(got), len(ids))
	}
	for i, want := range ids {
		if got[i] != want {
			t.Errorf("point %d: got %q, want %q", i, got[i], want)
		}
	}
}

func TestWAL_WriteAndDrainFIFO(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	points := []string{"A", "B", "C", "D", "E"}
	for _, id := range points {
		if err := w.Write(walTestPoint(id)); err != nil {
			t.Fatalf("Write(%s): %v", id, err)
		}
	}

	ch := w.Drain(context.Background())
	var got []string
	for p := range ch {
		got = append(got, p.ID)
	}

	if len(got) != len(points) {
		t.Fatalf("drained %d points, want %d", len(got), len(points))
	}
	for i, want := range points {
		if got[i] != want {
			t.Errorf("point %d: got %q, want %q", i, got[i], want)
		}
	}
}

func TestWAL_SegmentRotation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write enough data to cause segment rotation (10MB segment threshold).
	// Use 500 metrics with longer keys to produce ~10KB per point,
	// so 2000 points ≈ 20MB, guaranteeing at least one rotation.
	bigMetrics := make(map[string]float64)
	for i := range 500 {
		bigMetrics[fmt.Sprintf("metric_%04d_padding_for_size", i)] = float64(i)
	}

	const totalPoints = 2000
	for i := range totalPoints {
		p := normalizer.TelemetryPoint{
			ID:          string(rune(i)),
			AgentID:     "agent-test",
			AdapterName: "test",
			Source:      "src",
			RackID:      "rack",
			Timestamp:   time.Now(),
			ReceivedAt:  time.Now(),
			Metrics:     bigMetrics,
		}
		if err := w.Write(p); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// Close and count segments on disk.
	w.Close()

	entries, _ := os.ReadDir(dir)
	segCount := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".seg" {
			segCount++
		}
	}

	if segCount < 2 {
		t.Errorf("expected at least 2 segments after rotation, got %d", segCount)
	}

	// Reopen and drain — all points should come back.
	w2, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("reopening WAL: %v", err)
	}
	ch := w2.Drain(context.Background())
	count := 0
	for range ch {
		count++
	}
	if count != totalPoints {
		t.Errorf("drained %d points, want %d", count, totalPoints)
	}
}

func TestWAL_OldestSegmentDroppedOnOverflow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Very small max to force segment dropping.
	w, err := NewWAL(dir, 1024, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write enough points to exceed 1KB and force drops.
	for i := range 100 {
		w.Write(walTestPoint(string(rune('A' + i%26))))
	}

	// WAL should still be functional (didn't error out).
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify we're down to at most 1 segment (the active one is never pruned).
	entries, _ := os.ReadDir(dir)
	segCount := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".seg" {
			segCount++
		}
	}
	if segCount > 1 {
		t.Errorf("expected at most 1 segment after pruning, got %d", segCount)
	}
}

func TestWAL_PruneDoesNotDeleteActiveSegment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Very small max to force aggressive pruning.
	w, err := NewWAL(dir, 256, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write enough data to trigger prune attempts.
	for i := range 50 {
		w.Write(walTestPoint(string(rune('A' + i%26))))
	}

	// Active segment should still exist on disk.
	w.mu.Lock()
	activePath := ""
	if w.active != nil {
		activePath = w.active.path
	}
	w.mu.Unlock()

	if activePath == "" {
		t.Fatal("expected active segment to exist")
	}

	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		t.Fatalf("active segment %s was deleted by prune", activePath)
	}

	// Writes should still succeed after aggressive pruning.
	if err := w.Write(walTestPoint("final")); err != nil {
		t.Fatalf("Write after prune: %v", err)
	}

	w.Close()
}

func TestWAL_WritesAfterAggressivePrune(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Very small max so pruning is triggered on most writes.
	w, err := NewWAL(dir, 512, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write, triggering prune.
	for i := range 20 {
		if err := w.Write(walTestPoint(string(rune('A' + i%26)))); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	w.Close()

	// Reopen and drain — remaining data should be recoverable.
	w2, err := NewWAL(dir, 512, discardLogger())
	if err != nil {
		t.Fatalf("reopening WAL: %v", err)
	}

	ch := w2.Drain(context.Background())
	count := 0
	for range ch {
		count++
	}
	// Some points were pruned, but at least the latest ones survive.
	if count == 0 {
		t.Error("expected at least some points to survive prune, got 0")
	}
}

func TestWAL_CrashRecovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write some data, close (simulating crash by closing WAL).
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ids := []string{"X", "Y", "Z"}
	for _, id := range ids {
		if err := w.Write(walTestPoint(id)); err != nil {
			t.Fatalf("Write(%s): %v", id, err)
		}
	}
	w.Close()

	// Reopen — NewWAL should discover existing segments.
	w2, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("reopening WAL: %v", err)
	}

	if !w2.HasData() {
		t.Error("reopened WAL should HasData()")
	}

	ch := w2.Drain(context.Background())
	var got []string
	for p := range ch {
		got = append(got, p.ID)
	}

	if len(got) != len(ids) {
		t.Fatalf("drained %d points, want %d", len(got), len(ids))
	}
	for i, want := range ids {
		if got[i] != want {
			t.Errorf("point %d: got %q, want %q", i, got[i], want)
		}
	}
}

func TestWAL_CorruptedRecordSkipped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write 3 good records.
	w.Write(walTestPoint("A"))
	w.Write(walTestPoint("B"))
	w.Write(walTestPoint("C"))
	w.Close()

	// Corrupt the CRC of the second record in the segment file.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".seg" {
			path := filepath.Join(dir, e.Name())
			data, _ := os.ReadFile(path)

			// Find second record: skip first record (4 byte len + payload + 4 byte CRC).
			firstLen := binary.BigEndian.Uint32(data[0:4])
			offset := 4 + int(firstLen) + 4 // start of second record

			// Corrupt the CRC of the second record.
			secondLen := binary.BigEndian.Uint32(data[offset : offset+4])
			crcOffset := offset + 4 + int(secondLen)
			data[crcOffset] ^= 0xFF // flip bits

			os.WriteFile(path, data, 0644)
			break
		}
	}

	// Reopen and drain — should get A and C, skip B.
	w2, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("reopening WAL: %v", err)
	}

	ch := w2.Drain(context.Background())
	var got []string
	for p := range ch {
		got = append(got, p.ID)
	}

	if len(got) != 2 {
		t.Fatalf("drained %d points, want 2 (A and C)", len(got))
	}
	if got[0] != "A" {
		t.Errorf("first point: got %q, want A", got[0])
	}
	if got[1] != "C" {
		t.Errorf("second point: got %q, want C", got[1])
	}
}

func TestWAL_CorruptPayloadRecovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write 5 good records.
	ids := []string{"A", "B", "C", "D", "E"}
	for _, id := range ids {
		if err := w.Write(walTestPoint(id)); err != nil {
			t.Fatalf("Write(%s): %v", id, err)
		}
	}
	w.Close()

	// Corrupt the 3rd record's payload bytes (flip bits, causing CRC mismatch).
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".seg" {
			path := filepath.Join(dir, e.Name())
			data, _ := os.ReadFile(path)

			// Skip to 3rd record.
			offset := 0
			for i := 0; i < 2; i++ {
				recLen := binary.BigEndian.Uint32(data[offset : offset+4])
				offset += 4 + int(recLen) + 4
			}

			// Corrupt payload bytes of the 3rd record (after the length header).
			thirdLen := binary.BigEndian.Uint32(data[offset : offset+4])
			for j := offset + 4; j < offset+4+int(thirdLen); j++ {
				data[j] ^= 0xFF
			}

			os.WriteFile(path, data, 0644)
			break
		}
	}

	// Reopen and drain — should recover A, B, D, E (skip C).
	w2, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("reopening WAL: %v", err)
	}

	ch := w2.Drain(context.Background())
	var got []string
	for p := range ch {
		got = append(got, p.ID)
	}

	want := []string{"A", "B", "D", "E"}
	if len(got) != len(want) {
		t.Fatalf("drained %d points %v, want %d %v", len(got), got, len(want), want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("point %d: got %q, want %q", i, got[i], w)
		}
	}
}

func TestWAL_EmptyDrainClosesImmediately(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ch := w.Drain(context.Background())

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel, got value")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drain channel not closed in time")
	}
}

func TestWAL_CloseNoError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	// Write at least one point so active segment exists.
	w.Write(walTestPoint("A"))

	if err := w.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestWAL_Stats(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	w.Write(walTestPoint("A"))
	w.Write(walTestPoint("B"))

	totalSize, segs, points := w.Stats()
	if totalSize <= 0 {
		t.Errorf("totalSize = %d, want > 0", totalSize)
	}
	if segs < 1 {
		t.Errorf("segments = %d, want >= 1", segs)
	}
	if points != 2 {
		t.Errorf("points = %d, want 2", points)
	}

	w.Close()
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	p := walTestPoint("roundtrip")
	data, err := encodeRecord(p)
	if err != nil {
		t.Fatalf("encodeRecord: %v", err)
	}

	// Create a temp file to use as reader.
	f, err := os.CreateTemp(t.TempDir(), "wal-test")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	f.Seek(0, 0)

	got, _, err := decodeRecord(f)
	if err != nil {
		t.Fatalf("decodeRecord: %v", err)
	}

	if got.ID != p.ID {
		t.Errorf("ID = %q, want %q", got.ID, p.ID)
	}
	if got.AgentID != p.AgentID {
		t.Errorf("AgentID = %q, want %q", got.AgentID, p.AgentID)
	}
	if !got.Timestamp.Equal(p.Timestamp) {
		t.Errorf("Timestamp mismatch")
	}
	f.Close()
}

// writeSegmentFile encodes records for the given IDs and writes them to path.
func writeSegmentFile(t *testing.T, path string, ids []string) {
	t.Helper()
	var data []byte
	for _, id := range ids {
		d, err := encodeRecord(walTestPoint(id))
		if err != nil {
			t.Fatalf("encodeRecord(%s): %v", id, err)
		}
		data = append(data, d...)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestWAL_CancelledDrainPreservesSegments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Drain() uses a 256-buffer channel. We put 100 records in segment 0
	// and 300 in segments 1 and 2. After we read 100 records, the goroutine
	// has pushed at most 100+256=356 records total: seg0 (100) fully drained
	// plus at most 256 of seg1's 300 records. Cancel fires while the goroutine
	// is blocked mid-segment-1, so seg1 and seg2 are preserved.
	const seg0Recs = 100
	segRecords := []int{seg0Recs, 300, 300}
	for seg, nRecs := range segRecords {
		name := fmt.Sprintf("wal-%020d.seg", seg)
		ids := make([]string, nRecs)
		for r := range nRecs {
			ids[r] = fmt.Sprintf("s%d-r%03d", seg, r)
		}
		writeSegmentFile(t, filepath.Join(dir, name), ids)
	}

	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := w.Drain(ctx)

	// Read all of segment 0's records so the goroutine fully drains and
	// removes it. Context is still active, so the select always picks ch<-.
	for i := 0; i < seg0Recs; i++ {
		if _, ok := <-ch; !ok {
			t.Fatalf("channel closed early after %d records", i)
		}
	}

	// Cancel while the goroutine is processing segment 1.
	cancel()

	// Drain remaining buffered records so the goroutine can exit.
	for range ch {
	}

	// Segment 0 should be removed (fully drained before cancel).
	seg0 := filepath.Join(dir, fmt.Sprintf("wal-%020d.seg", 0))
	if _, err := os.Stat(seg0); !os.IsNotExist(err) {
		t.Errorf("segment 0 should be removed after successful drain, stat err: %v", err)
	}

	// Segments 1 and 2 should still exist on disk.
	for _, s := range []int{1, 2} {
		path := filepath.Join(dir, fmt.Sprintf("wal-%020d.seg", s))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("segment %d should be preserved on disk after cancelled drain", s)
		}
	}
}

func TestWAL_PreservedSegmentsRecoveredOnReopen(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// 3 segments with 300 records each (total 900). Drain() buffer is 256,
	// so the goroutine blocks mid-segment-0. Cancel → all 3 preserved.
	const recsPerSeg = 300
	for seg := 0; seg < 3; seg++ {
		name := fmt.Sprintf("wal-%020d.seg", seg)
		ids := make([]string, recsPerSeg)
		for r := range recsPerSeg {
			ids[r] = fmt.Sprintf("s%d-r%03d", seg, r)
		}
		writeSegmentFile(t, filepath.Join(dir, name), ids)
	}

	// First open: drain with cancel to simulate interrupted shutdown.
	w, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := w.Drain(ctx)
	cancel()
	for range ch {
	}

	// All 3 segments should be preserved (goroutine blocked in seg0).
	for seg := 0; seg < 3; seg++ {
		path := filepath.Join(dir, fmt.Sprintf("wal-%020d.seg", seg))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatalf("segment %d should be preserved after cancelled drain", seg)
		}
	}

	// Reopen WAL — preserved segments should be discovered and fully drained.
	w2, err := NewWAL(dir, 100<<20, discardLogger())
	if err != nil {
		t.Fatalf("reopening WAL: %v", err)
	}

	if !w2.HasData() {
		t.Fatal("reopened WAL should have data from preserved segments")
	}

	ch2 := w2.Drain(context.Background())
	count := 0
	for range ch2 {
		count++
	}

	if count != 3*recsPerSeg {
		t.Errorf("drained %d records, want %d", count, 3*recsPerSeg)
	}
}
