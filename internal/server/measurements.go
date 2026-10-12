package server

import (
	"context"
	"harbor/internal/model"
	"harbor/internal/pier"
	"net/http"
	"time"
)

func (s *Server) measurePier(w http.ResponseWriter, r *http.Request) {
	ship, ok := s.ship(w, r)
	if !ok {
		return
	}
	var body struct{}
	if err := decode(w, r, &body); err != nil {
		fail(w, 422, "validation", "Expected an empty measurement request.")
		return
	}
	if err := pier.VerifyRegistered(ship.PierPath, false); err != nil {
		fail(w, 409, "measurement", "Unable to measure this pier. Check that its folder is available and try again.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	size, err := pier.MeasureBytes(ctx, ship.PierPath)
	if err != nil {
		fail(w, 409, "measurement", "Unable to measure this pier. Check that its folder is available and try again.")
		return
	}
	if err := s.store.RecordMeasurement(r.Context(), ship.ID, size, time.Now()); err != nil {
		fail(w, 500, "storage", "Unable to save the pier measurement.")
		return
	}
	ship, ok = s.ship(w, r)
	if ok {
		s.Events.Publish(model.Event{Type: "ship", ShipID: ship.ID, Data: ship})
		respond(w, 200, ship)
	}
}
