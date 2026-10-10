//go:build darwin

package scheduler

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func parseMacAvailable(b []byte) (uint64, error) {
	page := regexp.MustCompile(`page size of ([0-9]+) bytes`).FindSubmatch(b)
	if len(page) != 2 {
		return 0, errors.New("host memory page size unavailable")
	}
	size, _ := strconv.ParseUint(string(page[1]), 10, 64)
	var pages uint64
	for _, line := range strings.Split(string(b), "\n") {
		for _, label := range []string{"Pages free:", "Pages inactive:", "Pages speculative:"} {
			if strings.HasPrefix(line, label) {
				n, e := strconv.ParseUint(strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, label)), "."), 10, 64)
				if e != nil {
					return 0, e
				}
				pages += n
			}
		}
	}
	if pages == 0 {
		return 0, errors.New("available host memory unavailable")
	}
	return pages * size, nil
}
func hostResources(ctx context.Context) (Resources, error) {
	b, err := exec.CommandContext(ctx, "/usr/bin/vm_stat").Output()
	if err != nil {
		return Resources{}, errors.New("host memory could not be read")
	}
	available, err := parseMacAvailable(b)
	if err != nil {
		return Resources{}, err
	}
	b, err = exec.CommandContext(ctx, "/usr/sbin/sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return Resources{}, err
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return Resources{}, err
	}
	b, err = exec.CommandContext(ctx, "/usr/sbin/sysctl", "-n", "machdep.cpu.brand_string").Output()
	return Resources{TotalMemory: total, AvailableMemory: available, Hardware: strings.TrimSpace(string(b))}, err
}
