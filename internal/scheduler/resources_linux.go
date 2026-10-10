//go:build linux

package scheduler

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

func parseLinuxMemory(b []byte) (uint64, uint64, error) {
	v := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 {
			n, e := strconv.ParseUint(f[1], 10, 64)
			if e == nil {
				v[strings.TrimSuffix(f[0], ":")] = n * 1024
			}
		}
	}
	if v["MemTotal"] == 0 || v["MemAvailable"] == 0 {
		return 0, 0, errors.New("available host memory could not be read")
	}
	return v["MemTotal"], v["MemAvailable"], nil
}
func hostResources(ctx context.Context) (Resources, error) {
	if err := ctx.Err(); err != nil {
		return Resources{}, err
	}
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Resources{}, err
	}
	total, available, err := parseLinuxMemory(b)
	hardware, _ := os.ReadFile("/sys/firmware/devicetree/base/model")
	return Resources{TotalMemory: total, AvailableMemory: available, Hardware: strings.TrimSpace(strings.TrimRight(string(hardware), "\x00"))}, err
}
