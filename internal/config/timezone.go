package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func hostTimezone() string {
	candidates := []string{strings.TrimPrefix(os.Getenv("TZ"), ":")}
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		candidates = append(candidates, strings.TrimSpace(string(b)))
	}
	if p, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if i := strings.LastIndex(p, "zoneinfo/"); i >= 0 {
			candidates = append(candidates, p[i+len("zoneinfo/"):])
		}
	}
	for _, zone := range candidates {
		if zone != "" && zone != "Local" {
			if _, err := time.LoadLocation(zone); err == nil {
				return zone
			}
		}
	}
	return "UTC"
}
