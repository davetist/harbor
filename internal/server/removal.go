package server

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/store"
	"net/http"
)

func (s *Server) removeShip(w http.ResponseWriter, r *http.Request) {
	ship, ok := s.ship(w, r)
	if !ok {
		return
	}
	remover, ok := s.actions.(interface {
		RemoveShip(context.Context, model.ID) error
	})
	if !ok {
		fail(w, 503, "starting", "Harbor's process controller is not ready.")
		return
	}
	if err := remover.RemoveShip(r.Context(), ship.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, 404, "not_found", "Ship not found.")
		} else {
			fail(w, 409, "conflict", err.Error())
		}
		return
	}
	w.WriteHeader(204)
}
