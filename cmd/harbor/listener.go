package main

import (
	"harbor/internal/config"
	"net"
)

func configuredListener(cfg *config.Config) (net.Listener, error) {
	network := "tcp"
	host, _, _ := net.SplitHostPort(cfg.ListenAddress)
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	}
	listener, err := net.Listen(network, cfg.ListenAddress)
	if err != nil {
		return nil, err
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return nil, err
	}
	// Preserve the configured binding while validating the actual
	// assigned port, including when the configuration requested port zero.
	cfg.ListenAddress = net.JoinHostPort(host, port)
	return listener, nil
}
