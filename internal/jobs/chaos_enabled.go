//go:build harbor_chaos

package jobs

import (
	"fmt"
	"os"
	"syscall"
)

// Only present in explicitly tagged native failure-window test binaries.
func testAfterUtilityExit() {
	if os.Getenv("HARBOR_CHAOS_AFTER_EXIT") == "1" {
		fmt.Fprintln(os.Stderr, "CHAOS: paused after durable utility exit, before job completion")
		syscall.Kill(os.Getpid(), syscall.SIGSTOP)
	}
}

func testAfterUtilitySpawn() {
	if os.Getenv("HARBOR_CHAOS_AFTER_SPAWN") == "1" {
		fmt.Fprintln(os.Stderr, "CHAOS: paused after durable utility identity, before wait")
		syscall.Kill(os.Getpid(), syscall.SIGSTOP)
	}
}
