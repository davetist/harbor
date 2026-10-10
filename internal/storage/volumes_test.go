package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testUUID = "11111111-aaaa-bbbb-cccc-555555555555"

func testMount(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGuardAcceptsMatchingVolumeWithoutCreatingData(t *testing.T) {
	point := testMount(t)
	path := filepath.Join(point, "not-created", "piers")
	lookup := func(context.Context, string) (volumeInfo, error) {
		return volumeInfo{point, strings.ToUpper(testUUID)}, nil
	}
	g, err := acquire(context.Background(), []Volume{{point, testUUID}}, []string{path}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	fd := g.directories[0]
	g.Close()
	if _, err := fd.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("volume descriptor was not closed", err)
	}
	if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("validation created a destination", err)
	}
}

func TestGuardRejectsWrongOrUnavailableIdentity(t *testing.T) {
	point := testMount(t)
	path := filepath.Join(point, "not-created")
	for _, tc := range []struct {
		name string
		info volumeInfo
		err  error
		want string
	}{
		{"directory-on-parent-filesystem", volumeInfo{"/", testUUID}, nil, "not mounted"},
		{"replacement-drive", volumeInfo{point, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, nil, "different filesystem UUID"},
		{"missing-UUID", volumeInfo{point, ""}, nil, "different filesystem UUID"},
		{"metadata-failure", volumeInfo{}, errors.New("metadata unavailable"), "could not be verified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := acquire(context.Background(), []Volume{{point, testUUID}}, []string{path}, func(context.Context, string) (volumeInfo, error) { return tc.info, tc.err })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal("unsafe volume accepted", err)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("refusal created a destination", err)
			}
		})
	}
}

func TestGuardRejectsReplacementDuringOpen(t *testing.T) {
	point := testMount(t)
	calls := 0
	_, err := acquire(context.Background(), []Volume{{point, testUUID}}, []string{point}, func(context.Context, string) (volumeInfo, error) {
		calls++
		if calls == 1 {
			return volumeInfo{point, testUUID}, nil
		}
		return volumeInfo{point, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "different filesystem UUID") || calls != 2 {
		t.Fatal("replacement after first check accepted", err, calls)
	}
}

func TestGuardRejectsSymlinkEscape(t *testing.T) {
	point, outside := testMount(t), testMount(t)
	link := filepath.Join(point, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := acquire(context.Background(), []Volume{{point, testUUID}}, []string{filepath.Join(link, "data")}, func(context.Context, string) (volumeInfo, error) { return volumeInfo{point, testUUID}, nil })
	if err == nil || !strings.Contains(err.Error(), "outside its required filesystem") {
		t.Fatal("symlink escape accepted", err)
	}
}

func TestCancelledGuardRefusesCleanly(t *testing.T) {
	point := testMount(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := acquire(ctx, []Volume{{point, testUUID}}, []string{point}, func(context.Context, string) (volumeInfo, error) {
		t.Fatal("lookup after cancellation")
		return volumeInfo{}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}

func TestConfigurationRejectsUnusedDuplicateAndInvalidPins(t *testing.T) {
	for _, volumes := range [][]Volume{
		{{"relative", testUUID}},
		{{"/mnt/harbor", "../device"}},
		{{"/mnt/harbor", ""}},
		{{"/mnt/harbor", testUUID}, {"/mnt/harbor", testUUID}},
		{{"/mnt/elsewhere", testUUID}},
		{{"/", testUUID}}, // A root-filesystem pin cannot cover a missing external mount.
	} {
		if err := ValidateConfiguration(volumes, []string{"/mnt/harbor/data"}); err == nil {
			t.Fatal("invalid external-storage policy accepted", volumes)
		}
	}
}
