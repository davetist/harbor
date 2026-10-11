package onboarding

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/pier"
)

func (s *Service) Retry(ctx context.Context, id, upload model.ID) (model.ID, error) {
	j, err := s.Store.GetJob(ctx, id)
	if err != nil {
		return "", err
	}
	if !s.Store.CanRetryPrelaunch(ctx, j) {
		return "", errors.New("setup cannot safely be retried; inspect its recorded outcome")
	}
	ship, err := s.Store.GetShip(ctx, j.ShipID)
	if err != nil {
		return "", err
	}
	if err = pier.VerifyRegistered(ship.PierPath, j.Kind != "import"); err != nil {
		return "", err
	}
	if j.Kind == "boot_keyfile" {
		if _, err = s.Uploads.KeyPath(upload); err != nil {
			return "", err
		}
	}
	next, err := s.Store.RetryPrelaunch(ctx, id, upload)
	return next.ID, err
}
