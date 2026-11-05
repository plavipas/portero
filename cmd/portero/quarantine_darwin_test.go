//go:build darwin

package main

import "testing"

func TestQuarantineBlocks(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"0081;00000000;Safari;", true},
		{"0083;5f0c1a2b;Safari;ABCD", true},
		{"03c1;5f0c1a2b;Safari;ABCD\n", false},
		{"03e1;5f0c1a2b;Safari;ABCD", false},
		{"0381;5f0c1a2b;Safari;ABCD", true}, // flag 0x40 not set
		{"00c1;5f0c1a2b;Safari;", false},
		{"garbage", true},
		{"", true},
		{";x", true},
	}
	for _, tt := range tests {
		if got := quarantineBlocks(tt.value); got != tt.want {
			t.Errorf("quarantineBlocks(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
