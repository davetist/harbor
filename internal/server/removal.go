package server

import (
	"context"
	"encoding/json"
	"errors"
	"harbor/internal/model"
	"harbor/internal/storage"
	"harbor/internal/store"
	"io"
	"net/http"
)

func (s *Server) removeShip(w http.ResponseWriter, r *http.Request) {
	ship, ok := s.ship(w, r)
	if !ok {
		return
	}
	var options struct {
		DeletePier      bool   `json:"deletePier"`
		ConfirmPierPath string `json:"confirmPierPath"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil && err != io.EOF {
		fail(w, 400, "invalid_request", "Invalid ship removal options.")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		fail(w, 400, "invalid_request", "Removal requires one JSON object.")
		return
	}
	if options.DeletePier {
		if options.ConfirmPierPath != ship.PierPath {
			fail(w, 400, "confirmation_required", "Confirm the exact registered pier path before permanently deleting its files.")
			return
		}
		protected := append([]string{s.cfg.DataDir}, s.cfg.AllowedRoots...)
		for _, volume := range s.cfg.RequiredVolumes {
			protected = append(protected, volume.MountPoint)
		}
		for _, path := range protected {
			inside, err := storage.ContainsPath(ship.PierPath, path)
			if err != nil || inside {
				fail(w, 409, "conflict", "Pier folder contains Harbor storage or a configured root; it cannot be permanently deleted.")
				return
			}
		}
		guard, err := storage.Acquire(r.Context(), s.cfg.RequiredVolumes, append(append([]string{s.cfg.DataDir}, s.cfg.AllowedRoots...), ship.PierPath))
		if err != nil {
			fail(w, 409, "storage", err.Error())
			return
		}
		defer guard.Close()
		remover, ok := s.actions.(interface {
			DeleteShip(context.Context, model.ID, string) error
		})
		if !ok {
			fail(w, 503, "starting", "Harbor's process controller is not ready.")
			return
		}
		if err = remover.DeleteShip(r.Context(), ship.ID, options.ConfirmPierPath); err != nil {
			fail(w, 409, "conflict", err.Error())
			return
		}
		w.WriteHeader(204)
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
