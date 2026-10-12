package runtime

import (
	"fmt"
	"harbor/internal/model"
	"os"
	"strings"
	"testing"
)

func TestFourPlatformMappings(t *testing.T) {
	for _, v := range []struct{ os, arch, want string }{{"darwin", "amd64", "macos-x86_64"}, {"darwin", "arm64", "macos-aarch64"}, {"linux", "amd64", "linux-x86_64"}, {"linux", "arm64", "linux-aarch64"}} {
		p, e := DetectPlatform(v.os, v.arch)
		if e != nil || p.Artifact != v.want {
			t.Fatalf("%+v %v", p, e)
		}
	}
	if _, e := DetectPlatform("windows", "amd64"); e == nil {
		t.Fatal("unexpected platform")
	}
}
func TestEveryShipLaunchIncludesHTTPBind(t *testing.T) {
	for _, bind := range []string{"127.0.0.1", "192.168.50.10"} {
		s := model.Ship{PierPath: "/test/pier", LoomExponent: 31, HTTPBindAddress: bind, WebPort: 8080}
		a, e := BuildArgs(s, "start")
		if e != nil {
			t.Fatal(e)
		}
		joined := strings.Join(a, " ")
		if !strings.Contains(joined, "--http-ip "+bind) || !strings.Contains(joined, "--http-port 8080") {
			t.Fatal(joined)
		}
	}
}
func TestUnsupportedLoomRejected(t *testing.T) {
	_, e := BuildArgs(model.Ship{PierPath: "/test/pier", LoomExponent: 99, HTTPBindAddress: "127.0.0.1", WebPort: 8080}, "start")
	if e == nil {
		t.Fatal("invalid loom accepted")
	}
}

func TestStartArgsAllowUnassignedHTTPPortAndPort80(t *testing.T) {
	for _, port := range []int{0, 80} {
		t.Run(fmt.Sprint(port), func(t *testing.T) {
			args, err := BuildArgs(model.Ship{PierPath: "/test/pier", LoomExponent: 31, HTTPBindAddress: "127.0.0.1", WebPort: port}, "start")
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if port == 0 && strings.Contains(joined, "--http-port") {
				t.Fatal("unassigned port was passed to Vere", args)
			}
			if port == 80 && !strings.Contains(joined, "--http-port 80") {
				t.Fatal("port 80 was not passed to Vere", args)
			}
		})
	}
}
func TestMaintenanceReusesLoom(t *testing.T) {
	for _, op := range []model.Operation{"pack", "meld", "roll", "chop"} {
		a, e := BuildArgs(model.Ship{PierPath: "/test/pier", LoomExponent: 32}, op)
		if e != nil || !strings.Contains(strings.Join(a, " "), "--loom 32") {
			t.Fatalf("%v %v", a, e)
		}
	}
}

func TestForeignDockedBinaryRejected(t *testing.T) {
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	native, e := NativePlatform()
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidateBinary(binary, native); e != nil {
		t.Fatal(e)
	}
	foreign := native
	if native.OS == "darwin" {
		foreign.OS = "linux"
	} else {
		foreign.OS = "darwin"
	}
	if e = ValidateBinary(binary, foreign); e == nil {
		t.Fatal("foreign runtime accepted")
	}
	foreign = native
	if native.Arch == "arm64" {
		foreign.Arch = "amd64"
	} else {
		foreign.Arch = "arm64"
	}
	if e = ValidateBinary(binary, foreign); e == nil {
		t.Fatal("foreign architecture accepted")
	}
}
