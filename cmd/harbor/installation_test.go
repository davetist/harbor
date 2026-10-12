package main

import "testing"

func TestInstallOptionsUseTheSameBootPolicyForLinux(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			opts, err := parseInstallOptions([]string{"--upgrade", "--config", "/private/config.json"}, platform)
			if err != nil {
				t.Fatal(err)
			}
			if opts.Headless != (platform == "linux") || !opts.Upgrade || opts.Config != "/private/config.json" {
				t.Fatalf("unexpected default installation options: %+v", opts)
			}
		})
	}
}

func TestInstallOptionsPreserveExplicitLoginOnlyMode(t *testing.T) {
	opts, err := parseInstallOptions([]string{"--headless=false", "--no-start"}, "linux")
	if err != nil || opts.Headless || !opts.NoStart {
		t.Fatalf("explicit options ignored: %+v %v", opts, err)
	}
}
