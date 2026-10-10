package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const StableVersion = "4.6"

var releaseHashes = map[string]string{
	"macos-aarch64": "dbe59fc917699ae0e03749fbe50ff2ab1cbc2d5b862c13585a4c64e7e9be4a37",
	"macos-x86_64":  "0bdbb881fb012b294d3dd8b3e8ce56eb9b55e33e8941d4517ccf775ebaae681e",
	"linux-aarch64": "ca2fa503e542f941021e793190b0498d4fde8d9d8856b266eaebc65e52e2d088",
	"linux-x86_64":  "f332e96d66cb767f847fbf15e04b4f953fd078e610b449fbbc26629ea312a3ae",
}

type Resolver struct{ Dir string }

func Resolve(ctx context.Context, p Platform, version string) (Runtime, error) {
	base, e := os.UserCacheDir()
	if e != nil {
		return Runtime{}, e
	}
	return (Resolver{Dir: filepath.Join(base, "Harbor", "runtimes")}).Resolve(ctx, p, version)
}
func (r Resolver) Resolve(ctx context.Context, p Platform, version string) (Runtime, error) {
	v := Runtime{Version: version, Platform: p}
	if version != StableVersion || releaseHashes[p.Artifact] == "" {
		return v, errors.New("runtime is not in Harbor's verified release catalog")
	}
	dir := filepath.Join(r.Dir, version, p.Artifact)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return v, e
	}
	v.Path = filepath.Join(dir, "vere")
	if e := validateInstalled(v.Path, p); e == nil {
		v.Digest, _ = FileDigest(v.Path)
		return v, nil
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://github.com/urbit/vere/releases/download/vere-v"+version+"/"+p.Artifact+".tgz", nil)
	if e != nil {
		return v, e
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, e := client.Do(req)
	if e != nil {
		return v, errors.New("official runtime download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return v, fmt.Errorf("runtime download returned HTTP %d", resp.StatusCode)
	}
	archive, e := os.CreateTemp(dir, ".archive-")
	if e != nil {
		return v, e
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, 256<<20+1))
	if e != nil || n > 256<<20 {
		return v, errors.New("runtime download exceeds limit or was interrupted")
	}
	if hex.EncodeToString(hash.Sum(nil)) != releaseHashes[p.Artifact] {
		return v, errors.New("runtime publisher SHA-256 does not match")
	}
	archive.Seek(0, io.SeekStart)
	gz, e := gzip.NewReader(archive)
	if e != nil {
		return v, e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	h, e := tr.Next()
	if e != nil {
		return v, e
	}
	if h.Typeflag != tar.TypeReg || h.Name != "vere-v"+version+"-"+p.Artifact || h.Size > 256<<20 {
		return v, errors.New("unexpected runtime archive")
	}
	staged, e := os.CreateTemp(dir, ".vere-")
	if e != nil {
		return v, e
	}
	defer os.Remove(staged.Name())
	if _, e = io.Copy(staged, tr); e != nil {
		staged.Close()
		return v, e
	}
	if e = staged.Chmod(0700); e != nil {
		staged.Close()
		return v, e
	}
	if e = staged.Sync(); e != nil {
		staged.Close()
		return v, e
	}
	staged.Close()
	if _, e = tr.Next(); e != io.EOF {
		return v, errors.New("unexpected extra runtime archive entry")
	}
	if e = ValidateBinary(staged.Name(), p); e != nil {
		return v, e
	}
	digest, e := FileDigest(staged.Name())
	if e != nil {
		return v, e
	}
	// Sidecar binds the installed bytes to the artifact verified above.
	if e = os.Rename(staged.Name(), v.Path); e != nil {
		return v, e
	}
	if e = os.WriteFile(v.Path+".sha256", []byte(digest), 0600); e != nil {
		return v, e
	}
	v.Digest = digest
	return v, nil
}
func validateInstalled(path string, p Platform) error {
	if e := ValidateBinary(path, p); e != nil {
		return e
	}
	wanted, e := os.ReadFile(path + ".sha256")
	if e != nil {
		return e
	}
	actual, e := FileDigest(path)
	if e != nil {
		return e
	}
	if actual != strings.TrimSpace(string(wanted)) {
		return errors.New("installed runtime changed")
	}
	return nil
}
func FileDigest(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
