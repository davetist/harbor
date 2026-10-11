package installation

import (
	"context"
	"encoding/json"
	"errors"
	"harbor/internal/config"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

type Status struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
	Headless  bool   `json:"headless"`
	URL       string `json:"url,omitempty"`
	DataDir   string `json:"dataDir,omitempty"`
	Config    string `json:"config,omitempty"`
	Runner    string `json:"runner,omitempty"`
	Source    string `json:"source,omitempty"`
}

func localURL(c config.Config) (string, error) {
	host, port, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return "", err
	}
	if port == "0" {
		return "", errors.New("installed services need a fixed listen port")
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func (m *Manager) ready(ctx context.Context, url, dataDir string) bool {
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/api/v1/host", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var host struct {
		DataDir    string `json:"dataDir"`
		Platform   string `json:"platform"`
		ManagerPID int    `json:"managerPID"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&host) != nil {
		return false
	}
	return host.DataDir == dataDir && len(host.Platform) > len(m.Platform) && host.Platform[:len(m.Platform)+1] == m.Platform+"/" && host.ManagerPID > 0 && host.ManagerPID == m.servicePID(ctx)
}

func (m *Manager) waitReady(ctx context.Context, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	url, err := localURL(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	for {
		if m.ready(ctx, url, cfg.DataDir) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("Harbor has not become ready; check background-app permission and the Harbor service logs, then open it again")
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	r, err := m.readRecord()
	if os.IsNotExist(err) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	cfg, err := config.Load(r.Config)
	if err != nil {
		return Status{}, err
	}
	url, err := localURL(cfg)
	if err != nil {
		return Status{}, err
	}
	return Status{Installed: true, Running: m.ready(ctx, url, cfg.DataDir), Enabled: r.Enabled, Headless: r.Headless, URL: url, DataDir: cfg.DataDir, Config: r.Config, Runner: m.Paths.Binary, Source: r.Source}, nil
}
