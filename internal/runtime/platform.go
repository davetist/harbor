package runtime

import (
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"os"
	goruntime "runtime"
)

type Platform struct {
	OS       string
	Arch     string
	Artifact string
}
type Runtime struct {
	Path     string
	Version  string
	Platform Platform
	Digest   string
}

func DetectPlatform(goos, goarch string) (Platform, error) {
	osname := map[string]string{"darwin": "macos", "linux": "linux"}[goos]
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	if osname == "" || arch == "" {
		return Platform{}, errors.New("Harbor supports macOS and Linux on x86_64 and ARM64")
	}
	return Platform{goos, goarch, osname + "-" + arch}, nil
}
func ValidateBinary(path string, p Platform) error {
	st, e := os.Stat(path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() {
		return errors.New("runtime is not a regular executable")
	}
	switch p.OS {
	case "darwin":
		f, e := macho.Open(path)
		if e != nil {
			return errors.New("runtime is not a native macOS executable")
		}
		defer f.Close()
		want := macho.CpuArm64
		if p.Arch == "amd64" {
			want = macho.CpuAmd64
		}
		if f.Cpu != want {
			return errors.New("runtime architecture mismatch")
		}
	case "linux":
		f, e := elf.Open(path)
		if e != nil {
			return errors.New("runtime is not a native Linux executable")
		}
		defer f.Close()
		want := elf.EM_AARCH64
		if p.Arch == "amd64" {
			want = elf.EM_X86_64
		}
		if f.Machine != want {
			return errors.New("runtime architecture mismatch")
		}
	default:
		return fmt.Errorf("unsupported runtime platform %s", p.OS)
	}
	return nil
}
func NativePlatform() (Platform, error) { return DetectPlatform(goruntime.GOOS, goruntime.GOARCH) }
