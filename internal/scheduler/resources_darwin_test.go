//go:build darwin

package scheduler

import "testing"

func TestMacAvailableMemoryExcludesActiveAndWiredPages(t *testing.T) {
	b := []byte("Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 10.\nPages active: 9000.\nPages inactive: 20.\nPages speculative: 5.\nPages wired down: 8000.\n")
	got, err := parseMacAvailable(b)
	if err != nil || got != 35*16384 {
		t.Fatal("memory estimate wrong", got, err)
	}
}
