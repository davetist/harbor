//go:build linux

package scheduler

import "testing"

func TestLinuxAvailableMemoryUsesMemAvailable(t *testing.T) {
	total, available, err := parseLinuxMemory([]byte("MemTotal: 8000 kB\nMemFree: 100 kB\nMemAvailable: 3000 kB\n"))
	if err != nil || total != 8000*1024 || available != 3000*1024 {
		t.Fatal("memory estimate wrong", total, available, err)
	}
}
