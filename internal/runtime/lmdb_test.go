package runtime

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func mapTestPier(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if err := os.MkdirAll(filepath.Join(p, ".urb/log"), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}

func mapTestDatabase(t *testing.T, pier, relative string, size int64) {
	t.Helper()
	path := filepath.Join(pier, ".urb/log", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Sparse logical size models a large database without using that disk space.
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
}

func TestLMDBMapOnEveryStart(t *testing.T) {
	p := mapTestPier(t)
	mapTestDatabase(t, p, "data.mdb", 16384)
	mapTestDatabase(t, p, "0i123/data.mdb", 1<<20)
	for i := 0; i < 3; i++ {
		args, err := LMDBMapArgs(context.Background(), p, false)
		if err != nil || strings.Join(args, " ") != "--lmdb-map-size 68719476736" {
			t.Fatalf("start %d: %v %v", i, args, err)
		}
	}
}

func TestLMDBMapBootstrap(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "new-pier")
	args, err := LMDBMapArgs(context.Background(), missing, true)
	if err != nil || strings.Join(args, " ") != "--lmdb-map-size 68719476736" {
		t.Fatalf("bootstrap mapping: %v %v", args, err)
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatal("map selection created a pier", err)
	}
}

func TestLMDBMapChecksLargestDatabaseAndGrowthHeadroom(t *testing.T) {
	for _, relative := range []string{"data.mdb", "0i0/data.mdb", "0i999/data.mdb"} {
		t.Run(relative, func(t *testing.T) {
			p := mapTestPier(t)
			for _, v := range []struct{ database, mapping int64 }{
				{32 << 30, 64 << 30},
				{(32 << 30) + 1, 128 << 30},
				{40 << 30, 128 << 30},
				{100 << 30, 256 << 30},
			} {
				mapTestDatabase(t, p, relative, v.database)
				args, err := LMDBMapArgs(context.Background(), p, false)
				if err != nil || len(args) != 2 || args[1] != strconv.FormatInt(v.mapping, 10) {
					t.Fatalf("database %d: %v %v; want %d", v.database, args, err, v.mapping)
				}
			}
		})
	}
	// The largest independent environment sets the shared runtime option;
	// snapshots and the sum of separate epochs do not set the map size.
	p := mapTestPier(t)
	mapTestDatabase(t, p, "0i0/data.mdb", 20<<30)
	mapTestDatabase(t, p, "0i123/data.mdb", 40<<30)
	mapTestDatabase(t, p, "image.bin", 300<<30)
	args, err := LMDBMapArgs(context.Background(), p, false)
	if err != nil || len(args) != 2 || args[1] != "137438953472" {
		t.Fatalf("largest independent database: %v %v", args, err)
	}
	// Recheck disk state instead of reusing a cached choice on the next start.
	mapTestDatabase(t, p, "0i123/data.mdb", 100<<30)
	args, err = LMDBMapArgs(context.Background(), p, false)
	if err != nil || len(args) != 2 || args[1] != "274877906944" {
		t.Fatalf("database growth not reflected: %v %v", args, err)
	}
}

func TestLMDBMapSizeBoundariesAndOverflow(t *testing.T) {
	for _, v := range []struct{ database, mapping int64 }{
		{0, 64 << 30},
		{32 << 30, 64 << 30},
		{(32 << 30) + 1, 128 << 30},
		{64 << 30, 128 << 30},
		{(64 << 30) + 1, 256 << 30},
		{256 << 30, 512 << 30},
		{300 << 30, 1024 << 30},
		{1 << 61, 1 << 62},
	} {
		got, err := lmdbMapSize(v.database)
		if err != nil || got != v.mapping || got/2 < v.database || got&(got-1) != 0 {
			t.Fatalf("size %d: %d %v; want %d", v.database, got, err, v.mapping)
		}
	}
	for _, size := range []int64{-1, (1 << 61) + 1, math.MaxInt64 / 2, math.MaxInt64} {
		if got, err := lmdbMapSize(size); err == nil || got != 0 {
			t.Fatalf("invalid/overflowing size %d accepted: %d %v", size, got, err)
		}
	}
}

func TestLMDBMapRefusesUncheckedStorage(t *testing.T) {
	for _, path := range []string{".urb", ".urb/log", ".urb/log/data.mdb", ".urb/log/0i123"} {
		t.Run(path, func(t *testing.T) {
			p := mapTestPier(t)
			target := filepath.Join(p, path)
			if err := os.RemoveAll(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), target); err != nil {
				t.Fatal(err)
			}
			if args, err := LMDBMapArgs(context.Background(), p, false); err == nil || len(args) != 0 {
				t.Fatalf("unchecked symlink accepted: %v %v", args, err)
			}
		})
	}
	p := mapTestPier(t)
	if err := os.Mkdir(filepath.Join(p, ".urb/log/data.mdb"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := LMDBMapArgs(context.Background(), p, false); err == nil {
		t.Fatal("non-file database accepted")
	}
	if _, err := LMDBMapArgs(context.Background(), filepath.Join(t.TempDir(), "missing"), false); err == nil {
		t.Fatal("missing existing pier accepted")
	}
	if _, err := LMDBMapArgs(context.Background(), p, true); err == nil {
		t.Fatal("existing destination accepted as virgin")
	}
}

func TestLMDBMapCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	args, err := LMDBMapArgs(ctx, mapTestPier(t), false)
	if !errors.Is(err, context.Canceled) || len(args) != 0 {
		t.Fatalf("cancelled map scan continued: %v %v", args, err)
	}
}
