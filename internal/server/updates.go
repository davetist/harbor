package server

import (
	"harbor/internal/updates"
	"net/http"
)

// SetVersion supplies the running binary's build version before serving.
func (s *Server) SetVersion(version string) {
	s.version = version
	s.updateCheck = updates.New(version).Check
}

func (s *Server) updateRoutes() {
	s.mux.HandleFunc("GET /api/v1/updates", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, s.updateCheck(r.Context(), false))
	})
	s.mux.HandleFunc("POST /api/v1/updates/check", func(w http.ResponseWriter, r *http.Request) {
		if err := decode(w, r, &struct{}{}); err != nil {
			fail(w, 422, "updates", "Invalid update check request.")
			return
		}
		respond(w, 200, s.updateCheck(r.Context(), true))
	})
}
