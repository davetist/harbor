package server

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/supervisor"
	"net/http"
)

func (s *Server) interrupt(w http.ResponseWriter, r *http.Request) {
	ship, ok := s.ship(w, r)
	if !ok {
		return
	}
	var body struct{}
	if err := decode(w, r, &body); err != nil {
		fail(w, 422, "validation", "Expected an empty action object.")
		return
	}
	controller, ok := s.actions.(interface {
		InterruptShip(context.Context, model.ID) error
	})
	if !ok {
		fail(w, 503, "starting", "Harbor's process controller is not ready.")
		return
	}
	if err := controller.InterruptShip(r.Context(), ship.ID); err != nil {
		code := "conflict"
		if errors.Is(err, supervisor.ErrWebResponsive) {
			code = "ship_responsive"
		}
		fail(w, 409, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
