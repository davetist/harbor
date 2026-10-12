package onboarding

import (
	"context"
	"harbor/internal/config"
	"harbor/internal/store"
	"path/filepath"
	"testing"
)

func TestRegistrationLeavesHTTPPortUnassigned(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := Service{Store: st, Config: config.Config{AllowedRoots: []string{root}, DefaultShipHTTPBindAddress: "127.0.0.1", Timezone: "UTC"}}
	ctx := context.Background()
	for _, name := range []string{"first-comet", "second-comet"} {
		if _, err := s.CreateComet(ctx, CometRequest{FolderName: name, LoomExponent: 31}); err != nil {
			t.Fatal(err)
		}
	}
	ships, err := st.ListShips(ctx)
	if err != nil || len(ships) != 2 {
		t.Fatalf("registrations: %+v, %v", ships, err)
	}
	for _, ship := range ships {
		if ship.WebPort != 0 {
			t.Fatalf("registration assigned port %d before starting the ship", ship.WebPort)
		}
	}
}
