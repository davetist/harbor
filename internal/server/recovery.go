package server

import (
	"context"
	"harbor/internal/model"
	"net/http"
)

func (s *Server) recoveryRoutes() {
	s.mux.HandleFunc("POST /api/v1/ships/{id}/recovery/verify", func(w http.ResponseWriter, r *http.Request) {
		ship, ok := s.ship(w, r)
		if !ok {
			return
		}
		var body struct{}
		if e := decode(w, r, &body); e != nil {
			fail(w, 422, "validation", "Expected an empty verification request.")
			return
		}
		verifier, ok := s.actions.(interface {
			VerifyAndReleaseHold(context.Context, model.ID) error
		})
		if !ok {
			fail(w, 503, "starting", "Recovery verification is unavailable.")
			return
		}
		if e := verifier.VerifyAndReleaseHold(r.Context(), ship.ID); e != nil {
			fail(w, 409, "recovery_hold", e.Error())
			return
		}
		w.WriteHeader(204)
	})
}
