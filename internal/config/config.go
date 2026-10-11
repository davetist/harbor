package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"harbor/internal/storage"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	RequiredVolumes            []storage.Volume `json:"requiredVolumes,omitempty"`
	MaxArchiveBytes            int64            `json:"maxArchiveBytes"`
	ListenAddress              string           `json:"listenAddress"`
	DefaultShipHTTPBindAddress string           `json:"defaultShipHTTPBindAddress"`
	AdvertisedHost             string           `json:"advertisedHost"`
	DataDir                    string           `json:"dataDir"`
	// AllowedRoots retains legacy default-folder and volume-pin configuration;
	// it does not restrict where a user can create or register a pier.
	AllowedRoots []string `json:"allowedRoots"`
	Timezone     string   `json:"timezone"`
}

func Load(path string) (Config, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	c := Config{ListenAddress: "0.0.0.0:18789", DefaultShipHTTPBindAddress: "0.0.0.0", DataDir: filepath.Join(base, "Harbor"), Timezone: hostTimezone(), MaxArchiveBytes: 16 << 30}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return c, err
		}
		defer f.Close()
		d := json.NewDecoder(io.LimitReader(f, 1<<20))
		d.DisallowUnknownFields()
		if err = d.Decode(&c); err != nil {
			return c, fmt.Errorf("config: %w", err)
		}
		if err = d.Decode(new(any)); err != io.EOF {
			return c, errors.New("config must contain one JSON object")
		}
	}
	host, port, err := net.SplitHostPort(c.ListenAddress)
	if err != nil || host == "" || port == "" {
		return c, errors.New("listenAddress requires an explicit address and port")
	}
	if ip := net.ParseIP(c.DefaultShipHTTPBindAddress); ip == nil || ip.To4() == nil {
		return c, errors.New("ship HTTP binding requires an IPv4 address")
	}
	if c.AdvertisedHost != "" && (net.ParseIP(c.AdvertisedHost) == nil) {
		for _, r := range c.AdvertisedHost {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
				return c, errors.New("advertisedHost must be a hostname or IP address")
			}
		}
	}
	if _, err = time.LoadLocation(c.Timezone); err != nil {
		return c, fmt.Errorf("timezone: %w", err)
	}
	if c.MaxArchiveBytes < 1 || c.MaxArchiveBytes > 64<<30 {
		return c, errors.New("maxArchiveBytes must be between 1 byte and 64 GiB")
	}
	if !filepath.IsAbs(c.DataDir) {
		return c, errors.New("dataDir must be absolute")
	}
	c.DataDir = filepath.Clean(c.DataDir)
	if len(c.AllowedRoots) == 0 {
		c.AllowedRoots = []string{filepath.Join(c.DataDir, "piers")}
	}
	for i, p := range c.AllowedRoots {
		if !filepath.IsAbs(p) {
			return c, errors.New("allowedRoots must be absolute")
		}
		c.AllowedRoots[i] = filepath.Clean(p)
	}
	for i := range c.RequiredVolumes {
		c.RequiredVolumes[i].MountPoint = filepath.Clean(c.RequiredVolumes[i].MountPoint)
	}
	if err = storage.ValidateConfiguration(c.RequiredVolumes, append([]string{c.DataDir}, c.AllowedRoots...)); err != nil {
		return c, err
	}
	return c, nil
}
