// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package buffer

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultSegMaxSize int64 = 10 << 20 // 10MB per segment

// segment represents a single WAL segment file on disk.
type segment struct {
	path   string
	seqNum uint64
	file   *os.File
	size   int64
}

// createSegment creates a new segment file in dir with the given sequence number.
func createSegment(dir string, seqNum uint64) (*segment, error) {
	name := fmt.Sprintf("wal-%020d.seg", seqNum)
	path := filepath.Join(dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("creating segment %s: %w", path, err)
	}

	return &segment{
		path:   path,
		seqNum: seqNum,
		file:   f,
		size:   0,
	}, nil
}

// openSegmentForRead opens an existing segment file for reading.
func openSegmentForRead(path string) (*segment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening segment %s: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat segment %s: %w", path, err)
	}

	return &segment{
		path: path,
		file: f,
		size: info.Size(),
	}, nil
}

// append writes raw bytes to the segment and updates the tracked size.
func (s *segment) append(data []byte) error {
	n, err := s.file.Write(data)
	s.size += int64(n)
	if err != nil {
		return fmt.Errorf("writing to segment %s: %w", s.path, err)
	}
	return nil
}

// sync flushes the segment to disk.
func (s *segment) sync() error {
	return s.file.Sync()
}

// close closes the segment file.
func (s *segment) close() error {
	return s.file.Close()
}

// remove closes and deletes the segment file.
func (s *segment) remove() error {
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("segment close: %w", err)
	}
	return os.Remove(s.path)
}
