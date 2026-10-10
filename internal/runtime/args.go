package runtime

import (
	"errors"
	"harbor/internal/model"
	"net"
	"path/filepath"
	"strconv"
)

func BuildArgs(ship model.Ship, op model.Operation) ([]string, error) {
	if !filepath.IsAbs(ship.PierPath) {
		return nil, errors.New("absolute pier path required")
	}
	if ship.LoomExponent < 30 || ship.LoomExponent > 32 {
		return nil, errors.New("loom size has not been validated for this runtime")
	}
	loom := strconv.Itoa(ship.LoomExponent)
	switch op {
	case "start":
		ip := net.ParseIP(ship.HTTPBindAddress)
		if ip == nil || ip.To4() == nil {
			return nil, errors.New("explicit IPv4 HTTP listener required")
		}
		if ship.WebPort < 1024 || ship.WebPort > 65535 {
			return nil, errors.New("ship port must be unprivileged")
		}
		return []string{"--loom", loom, "--http-ip", ship.HTTPBindAddress, "--http-port", strconv.Itoa(ship.WebPort), ship.PierPath}, nil
	case "replay":
		return []string{"play", "--full", "--loom", loom, ship.PierPath}, nil
	case "update":
		return []string{"prep", "--loom", loom, ship.PierPath}, nil
	case "pack", "meld", "roll", "chop":
		return []string{string(op), "--loom", loom, ship.PierPath}, nil
	default:
		return nil, errors.New("operation has no validated runtime arguments")
	}
}
