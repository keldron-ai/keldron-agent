// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package config

import (
	"testing"
)

func TestParseByteSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		// Basic suffixes.
		{name: "bytes", input: "100B", want: 100},
		{name: "kilobytes", input: "1KB", want: 1024},
		{name: "megabytes", input: "500MB", want: 500 * 1024 * 1024},
		{name: "gigabytes", input: "2GB", want: 2 * 1024 * 1024 * 1024},
		{name: "terabytes", input: "1TB", want: 1024 * 1024 * 1024 * 1024},

		// Case insensitive.
		{name: "lowercase mb", input: "500mb", want: 500 * 1024 * 1024},
		{name: "mixed case Mb", input: "500Mb", want: 500 * 1024 * 1024},
		{name: "lowercase kb", input: "10kb", want: 10 * 1024},
		{name: "lowercase gb", input: "1gb", want: 1024 * 1024 * 1024},

		// No suffix = bytes.
		{name: "no suffix", input: "1024", want: 1024},

		// Whitespace.
		{name: "leading whitespace", input: "  500MB", want: 500 * 1024 * 1024},
		{name: "trailing whitespace", input: "500MB  ", want: 500 * 1024 * 1024},
		{name: "space between", input: "500 MB", want: 500 * 1024 * 1024},

		// Zero.
		{name: "zero bytes", input: "0B", want: 0},
		{name: "zero megabytes", input: "0MB", want: 0},

		// Errors.
		{name: "empty string", input: "", wantErr: true},
		{name: "bad suffix", input: "100XB", wantErr: true},
		{name: "negative", input: "-10MB", wantErr: true},
		{name: "only suffix", input: "MB", wantErr: true},
		{name: "garbage", input: "hello", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseByteSize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseByteSize(%q) = %d, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseByteSize(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseByteSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
