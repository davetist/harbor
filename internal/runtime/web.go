package runtime

import (
	"context"
	"errors"
	"harbor/internal/model"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

var ErrWebUnresponsive = errors.New("the ship's web interface did not respond")

// CheckWeb tests public HTTP without adding a private dojo query to a busy
// worker. A timeout means busy or unreachable, not proof of a stuck event.
func CheckWeb(ctx context.Context, ship model.Ship) error {
	ip := net.ParseIP(ship.HTTPBindAddress)
	if ip == nil || ip.To4() == nil || (ship.WebPort != 80 && (ship.WebPort < 1024 || ship.WebPort > 65535)) {
		return errors.New("invalid saved ship HTTP listener")
	}
	address := ip.String()
	if ip.IsUnspecified() {
		address = "127.0.0.1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(address, strconv.Itoa(ship.WebPort))+"/~/login", nil)
	if err != nil {
		return err
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return ErrWebUnresponsive
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 400 {
		return ErrWebUnresponsive
	}
	return nil
}
