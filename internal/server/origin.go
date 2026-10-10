package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) allowed(r *http.Request) bool {
	host, port, e := net.SplitHostPort(r.Host)
	if e != nil {
		return false
	}
	listenHost, listenPort, _ := net.SplitHostPort(s.cfg.ListenAddress)
	if port != listenPort {
		return false
	}
	listenIP := net.ParseIP(listenHost)
	if listenIP != nil && listenIP.IsUnspecified() {
		// Match Host to the actual destination; network access policy is external.
		localHost := "127.0.0.1"
		if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			localHost, _, e = net.SplitHostPort(local.String())
			if e != nil {
				return false
			}
		} else if host != "127.0.0.1" && host != "localhost" {
			return false
		}
		if host != localHost && host != s.cfg.AdvertisedHost && !(net.ParseIP(localHost).IsLoopback() && host == "localhost") {
			return false
		}
	} else if host != listenHost && host != s.cfg.AdvertisedHost && !(listenIP != nil && listenIP.IsLoopback() && (host == "localhost" || host == "127.0.0.1" || host == "::1")) {
		return false
	}
	if r.Method == "GET" || r.Method == "HEAD" {
		return true
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if err != nil || u.Scheme != scheme || u.Host != r.Host || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return false
		}
	}
	return true
}
