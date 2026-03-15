// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package buffer

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"google.golang.org/protobuf/proto"
)

// WAL is a segmented write-ahead log that stores TelemetryPoints on disk.
// Records are written with a length prefix and CRC32 checksum for integrity.
type WAL struct {
	dir      string
	maxTotal int64
	mu       sync.Mutex
	segments []string // sorted segment paths (oldest first)
	active   *segment
	totalSz  int64
	nextSeq  uint64
	logger   *slog.Logger

	pointsStored atomic.Uint64
	segCount     atomic.Int32
}

// NewWAL creates or opens a WAL in the given directory. It scans for existing
// segments (crash recovery) and sorts them by sequence number.
func NewWAL(dir string, maxTotal int64, logger *slog.Logger) (*WAL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating WAL dir %s: %w", dir, err)
	}

	w := &WAL{
		dir:      dir,
		maxTotal: maxTotal,
		logger:   logger,
	}

	// Scan for existing segments.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading WAL dir %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".seg") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			w.logger.Warn("skipping unreadable segment", "path", path, "error", err)
			continue
		}
		w.segments = append(w.segments, path)
		w.totalSz += info.Size()

		// Parse sequence number from filename.
		var seq uint64
		if _, err := fmt.Sscanf(e.Name(), "wal-%d.seg", &seq); err == nil {
			if seq >= w.nextSeq {
				w.nextSeq = seq + 1
			}
		}
	}

	sort.Slice(w.segments, func(i, j int) bool {
		var si, sj uint64
		fmt.Sscanf(filepath.Base(w.segments[i]), "wal-%d.seg", &si)
		fmt.Sscanf(filepath.Base(w.segments[j]), "wal-%d.seg", &sj)
		return si < sj
	})
	w.segCount.Store(int32(len(w.segments)))

	return w, nil
}

// Write encodes a TelemetryPoint and appends it to the active WAL segment.
// If the total WAL size exceeds maxTotal, the oldest segment is dropped.
// If the active segment exceeds defaultSegMaxSize, it is rotated.
func (w *WAL) Write(p normalizer.TelemetryPoint) error {
	data, err := encodeRecord(p)
	if err != nil {
		return fmt.Errorf("encoding WAL record: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Drop oldest segments if over size limit.
	for w.totalSz+int64(len(data)) > w.maxTotal && len(w.segments) > 0 {
		oldest := w.segments[0]
		if w.active != nil && oldest == w.active.path {
			break // never prune the active segment
		}
		w.segments = w.segments[1:]
		info, err := os.Stat(oldest)
		if err == nil {
			w.totalSz -= info.Size()
		}
		if err := os.Remove(oldest); err != nil {
			w.logger.Warn("failed to remove oldest segment", "path", oldest, "error", err)
		} else {
			w.logger.Warn("dropped oldest WAL segment due to size limit", "path", oldest)
		}
		w.segCount.Store(int32(len(w.segments)))
	}

	// Rotate segment if needed.
	if w.active == nil || w.active.size >= defaultSegMaxSize {
		if err := w.rotateSegmentLocked(); err != nil {
			return err
		}
	}

	if err := w.active.append(data); err != nil {
		return err
	}

	if err := w.active.sync(); err != nil {
		return fmt.Errorf("fsync WAL segment: %w", err)
	}

	w.totalSz += int64(len(data))
	w.pointsStored.Add(1)
	return nil
}

// Drain returns a channel that yields all WAL points in FIFO order.
// Each segment is deleted after being fully read. Corrupted records are
// skipped with an error log. The channel is closed when all segments are
// drained.
func (w *WAL) Drain(ctx context.Context) <-chan normalizer.TelemetryPoint {
	ch := make(chan normalizer.TelemetryPoint, 256)

	w.mu.Lock()
	// Close and remove the active segment from write mode so it can be read.
	if w.active != nil {
		if err := w.active.sync(); err != nil {
			w.logger.Error("WAL drain: sync active segment failed", "error", err)
		}
		if err := w.active.close(); err != nil {
			w.logger.Error("WAL drain: close active segment failed", "error", err)
		}
		w.active = nil
	}
	// Take ownership of the segment list.
	segs := make([]string, len(w.segments))
	copy(segs, w.segments)
	w.segments = nil
	w.totalSz = 0
	w.segCount.Store(0)
	w.mu.Unlock()

	go func() {
		defer close(ch)
		for _, path := range segs {
			if err := w.drainSegment(ctx, path, ch); err != nil {
				w.logger.Error("draining WAL segment", "path", path, "error", err)
				break // stop — this and remaining segments are preserved on disk
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				w.logger.Warn("failed to remove drained WAL segment", "path", path, "error", err)
			}
		}
	}()

	return ch
}

func (w *WAL) drainSegment(ctx context.Context, path string, ch chan<- normalizer.TelemetryPoint) error {
	seg, err := openSegmentForRead(path)
	if err != nil {
		return err
	}
	defer seg.close()

	for {
		start, err := seg.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}

		p, payloadLen, err := decodeRecord(seg.file)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			w.logger.Warn("skipping corrupted WAL record", "path", path, "error", err)
			if payloadLen < 0 {
				w.logger.Error("cannot determine record boundary, abandoning segment", "path", path)
				return nil
			}
			// Seek to end of record: header(4) + payload(payloadLen) + CRC(4).
			if _, err := seg.file.Seek(start+4+int64(payloadLen)+4, io.SeekStart); err != nil {
				return nil
			}
			continue
		}

		select {
		case ch <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// HasData returns true if the WAL has any segments (including the active one).
func (w *WAL) HasData() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.segments) > 0 || (w.active != nil && w.active.size > 0)
}

// Close fsyncs the active segment and closes all files.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.active != nil {
		if err := w.active.sync(); err != nil {
			return err
		}
		return w.active.close()
	}
	return nil
}

// Stats returns WAL statistics.
func (w *WAL) Stats() (totalSize int64, segments int, points uint64) {
	w.mu.Lock()
	totalSize = w.totalSz
	segments = len(w.segments)
	if w.active != nil {
		segments++
	}
	w.mu.Unlock()
	points = w.pointsStored.Load()
	return
}

// MaxTotal returns the maximum total WAL size in bytes.
func (w *WAL) MaxTotal() int64 {
	return w.maxTotal
}

func (w *WAL) rotateSegmentLocked() error {
	if w.active != nil {
		if err := w.active.sync(); err != nil {
			return fmt.Errorf("wal rotate sync: %w", err)
		}
		if err := w.active.close(); err != nil {
			return fmt.Errorf("wal rotate close: %w", err)
		}
	}

	seq := w.nextSeq
	w.nextSeq++

	seg, err := createSegment(w.dir, seq)
	if err != nil {
		return err
	}

	w.active = seg
	w.segments = append(w.segments, seg.path)
	w.segCount.Store(int32(len(w.segments)))
	return nil
}

// WAL record format:
//
//	[4 bytes] uint32 big-endian record length (of proto payload)
//	[N bytes] proto.Marshal(telemetryv1.TelemetryPoint)
//	[4 bytes] CRC32 checksum of the proto payload

func encodeRecord(p normalizer.TelemetryPoint) ([]byte, error) {
	pb := pointToProto(p)
	payload, err := proto.Marshal(pb)
	if err != nil {
		return nil, fmt.Errorf("marshalling telemetry point: %w", err)
	}

	buf := make([]byte, 4+len(payload)+4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(payload)))
	copy(buf[4:4+len(payload)], payload)
	binary.BigEndian.PutUint32(buf[4+len(payload):], crc32.ChecksumIEEE(payload))
	return buf, nil
}

// decodeRecord reads a single WAL record from r.
// The second return value (payloadLen) indicates how far the header was read:
//   - -1: length header was not read, or length exceeds sanity check
//   - >=0: payload length from the header (record occupies 4 + payloadLen + 4 bytes)
func decodeRecord(r io.Reader) (normalizer.TelemetryPoint, int, error) {
	var zero normalizer.TelemetryPoint

	// Read record length.
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return zero, -1, err
	}
	length := binary.BigEndian.Uint32(lenBuf[:])

	// Sanity check to avoid huge allocations on corrupted data.
	if length > 10<<20 { // 10MB max single record
		return zero, -1, fmt.Errorf("record length %d exceeds maximum", length)
	}

	payloadLen := int(length)

	// Read proto payload.
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return zero, payloadLen, fmt.Errorf("reading payload: %w", err)
	}

	// Read and verify CRC32.
	var crcBuf [4]byte
	if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
		return zero, payloadLen, fmt.Errorf("reading CRC: %w", err)
	}
	storedCRC := binary.BigEndian.Uint32(crcBuf[:])
	computedCRC := crc32.ChecksumIEEE(payload)
	if storedCRC != computedCRC {
		return zero, payloadLen, fmt.Errorf("CRC mismatch: stored=%x computed=%x", storedCRC, computedCRC)
	}

	// Unmarshal proto.
	pb := pointToProto(normalizer.TelemetryPoint{}) // allocate empty proto
	if err := proto.Unmarshal(payload, pb); err != nil {
		return zero, payloadLen, fmt.Errorf("unmarshalling payload: %w", err)
	}

	return protoToPoint(pb), payloadLen, nil
}
