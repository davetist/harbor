package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

var lockFixture = map[string][]byte{
	".urb/log/data.mdb":              []byte("durable metadata"),
	".urb/log/lock.mdb":              []byte("copied process and transaction state"),
	".urb/log/0i154/data.mdb":        []byte("durable events"),
	".urb/log/0i154/lock.mdb":        []byte("copied epoch readers"),
	".urb/put/lock.mdb":              []byte("application file"),
	".urb/log/not-an-epoch/lock.mdb": []byte("unrecognized durable file"),
}

func TestExportOmitsOnlyVereLMDBRuntimeLocks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	for path, data := range lockFixture {
		p := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Export(context.Background(), source, filepath.Join(root, "backup.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, f := range m.Files {
		present[f.Path] = true
	}
	for path := range lockFixture {
		runtimeLock := path == ".urb/log/lock.mdb" || path == ".urb/log/0i154/lock.mdb"
		if present[path] == runtimeLock {
			t.Fatalf("incorrect lock-file inclusion for %s", path)
		}
	}
}

func TestRestoreLegacyArchiveRegeneratesVereLMDBRuntimeLocks(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "old-backup.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	m := Manifest{Version: 1}
	for path, data := range lockFixture {
		sum := sha256.Sum256(data)
		m.Files = append(m.Files, File{Path: path, Kind: "file", Size: int64(len(data)), Mode: 0600, SHA256: hex.EncodeToString(sum[:])})
		if err = tw.WriteHeader(&tar.Header{Name: "pier/" + path, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err = tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = tw.WriteHeader(&tar.Header{Name: manifestName, Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(b))}); err != nil {
		t.Fatal(err)
	}
	if _, err = tw.Write(b); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{tw.Close, gz.Close, f.Close} {
		if err = close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = Validate(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "restored")
	if _, err = Restore(context.Background(), archive, dest); err != nil {
		t.Fatal(err)
	}
	for path, data := range lockFixture {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(path)))
		if path == ".urb/log/lock.mdb" || path == ".urb/log/0i154/lock.mdb" {
			if !os.IsNotExist(err) {
				t.Fatalf("restored stale native runtime lock: %s", path)
			}
		} else if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("durable file changed: %s: %v", path, err)
		}
	}
}
