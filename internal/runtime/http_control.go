package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"harbor/internal/model"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var identityPattern = regexp.MustCompile(`^~[a-z]{3,6}(?:-+[a-z]{6})*$`)
var plainCodePattern = regexp.MustCompile(`^[a-z]{6}(?:-[a-z]{6}){3}$`)

func loopbackControlURL(pier string) (string, error) {
	f, err := os.Open(filepath.Join(pier, ".http.ports"))
	if err != nil {
		return "", errors.New("private HTTP control port is unavailable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return "", errors.New("invalid private HTTP port file")
	}
	port := 0
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 3 || parts[2] != "loopback" {
			continue
		}
		n, e := strconv.Atoi(parts[0])
		if e != nil || n < 1 || n > 65535 || parts[1] != "insecure" || port != 0 {
			return "", errors.New("invalid private HTTP control port")
		}
		port = n
	}
	if port == 0 {
		return "", errors.New("private HTTP control port is unavailable")
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", port), nil
}
func privateHTTP(ctx context.Context, url, command string) (string, error) {
	// Fixed internal commands only. Never route through an environment proxy or
	// follow a redirect carrying the private query to another host.
	body := `{"sink":{"stdout":null},"source":{"dojo":` + strconv.Quote(command) + `}}`
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return "", errors.New("private query failed")
	}
	req.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return "", errors.New("private HTTP query failed")
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, maxSensitive+1))
	if err != nil || len(b) > maxSensitive || response.StatusCode != 200 {
		return "", errors.New("private HTTP response failed")
	}
	var result string
	if json.Unmarshal(b, &result) != nil {
		return "", errors.New("invalid private HTTP response")
	}
	return strings.TrimSpace(result), nil
}
func ReadIdentity(ctx context.Context, ship model.Ship) (string, error) {
	url, err := loopbackControlURL(ship.PierPath)
	if err != nil {
		return "", err
	}
	identity, err := privateHTTP(ctx, url, "our")
	if err != nil || !identityPattern.MatchString(identity) {
		return "", errors.New("private ship identity was not confirmed")
	}
	return identity, nil
}
func readHTTPAccessCode(ctx context.Context, ship model.Ship) (string, error) {
	url, err := loopbackControlURL(ship.PierPath)
	if err != nil {
		return "", err
	}
	identity, err := privateHTTP(ctx, url, "our")
	if err != nil || !identityPattern.MatchString(identity) {
		return "", errors.New("private ship identity was not confirmed")
	}
	expected := ship.Identity
	if expected == "" && identityPattern.MatchString(ship.Name) {
		expected = ship.Name
	}
	if expected != "" && identity != expected {
		return "", errors.New("private control port belongs to a different ship")
	}
	code, err := privateHTTP(ctx, url, "+code")
	if err != nil || !plainCodePattern.MatchString(code) {
		return "", errors.New("ship did not return a valid private access code")
	}
	return code, nil
}
func ReadAccessCode(_ context.Context, ship model.Ship) (string, error) {
	// Port's private HTTP route avoids conn.sock's reproduced early-close crash.
	// Drain the bounded reply even when the browser disconnects.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return readHTTPAccessCode(ctx, ship)
}
