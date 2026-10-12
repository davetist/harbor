package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"harbor/internal/installation"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

func parseInstallOptions(args []string, platform string) (installation.Options, error) {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	opts := installation.Options{}
	flags.StringVar(&opts.Config, "config", "", "existing absolute configuration path (defaults are created automatically)")
	flags.BoolVar(&opts.Upgrade, "upgrade", false, "gracefully update an existing installation, preserving its data and stopped state")
	flags.BoolVar(&opts.Headless, "headless", platform == "linux", "Linux: start at boot and keep running after logout (may request administrator approval)")
	flags.BoolVar(&opts.NoStart, "no-start", false, "prepare the installation without starting it")
	flags.StringVar(&opts.IfSource, "if-source", "", "update only an installation from this package path")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if len(flags.Args()) != 0 {
		return opts, errors.New("unexpected installation arguments")
	}
	return opts, nil
}

func installCommand(args []string) error {
	opts, err := parseInstallOptions(args, runtime.GOOS)
	if err != nil {
		return err
	}
	m, err := installation.New()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if opts.IfSource != "" {
		matches, e := m.FromSource(opts.IfSource)
		if e != nil {
			return e
		}
		if !matches {
			fmt.Println("Other-source Harbor installation preserved.")
			return nil
		}
	}
	if err = m.Install(ctx, opts); err != nil {
		return err
	}
	status, err := m.Status(ctx)
	if err != nil {
		return err
	}
	if status.Installed {
		if opts.IfSource != "" && status.Source != opts.IfSource {
			fmt.Println("Other-source Harbor installation preserved.")
			return nil
		}
		fmt.Println("Harbor installed. Your configuration and ship data are preserved.")
		if status.Running {
			printAddresses(status.URL)
		}
		if status.Headless {
			fmt.Println("Starts at boot and keeps running after logout.")
		}
	}
	return nil
}

func serviceCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: harbor service start | stop | status [--json] | uninstall")
	}
	action := args[0]
	flags := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	jsonOutput := flags.Bool("json", false, "machine-readable service status")
	ifSource := flags.String("if-source", "", "remove only an installation from this package path")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected service arguments")
	}
	m, err := installation.New()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch action {
	case "start":
		if err = m.Start(ctx); err == nil {
			fmt.Println("Harbor is running. Closing the interface leaves ships running.")
		}
	case "stop":
		if err = m.Stop(ctx); err == nil {
			fmt.Println("Harbor and its ships are stopped. Open Harbor to start it again.")
		}
	case "uninstall":
		if *ifSource != "" {
			matches, e := m.FromSource(*ifSource)
			if e != nil {
				return e
			}
			if !matches {
				fmt.Println("Other-source Harbor installation preserved.")
				return nil
			}
		}
		if err = m.Uninstall(ctx, *ifSource); err == nil {
			fmt.Println("Harbor's background service and installed runner removed. Ship data, exports and configuration preserved.")
		}
	case "status":
		status, e := m.Status(ctx)
		if e != nil {
			return e
		}
		if *jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(status)
		}
		if !status.Installed {
			fmt.Println("Harbor is not installed.")
			return nil
		}
		if status.Running {
			fmt.Println("Harbor is running.")
			printAddresses(status.URL)
		} else {
			fmt.Println("Harbor is stopped.")
		}
		fmt.Println("Ship data:", status.DataDir)
	default:
		return errors.New("unknown service action")
	}
	return err
}

func printAddresses(local string) {
	fmt.Println("Open", local)
	// Only show remote addresses for a wildcard listener. No discovery, network
	// configuration, or reachability promise is made by listing local interfaces.
	m, err := installation.New()
	if err != nil {
		return
	}
	status, err := m.Status(context.Background())
	if err != nil || status.Config == "" {
		return
	}
	// Reading the admitted configuration also avoids advertising a loopback-only host.
	contents, err := os.ReadFile(status.Config)
	if err != nil {
		return
	}
	var cfg struct {
		ListenAddress string `json:"listenAddress"`
	}
	if json.Unmarshal(contents, &cfg) != nil {
		return
	}
	if cfg.ListenAddress != "" {
		host, _, err := net.SplitHostPort(cfg.ListenAddress)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsUnspecified() {
			return
		}
	}
	_, port, err := net.SplitHostPort(local[len("http://"):])
	if err != nil {
		return
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			fmt.Println("On your trusted network: http://" + net.JoinHostPort(ip.String(), port))
		}
	}
}

func desktopCommand(args []string) error {
	flags := flag.NewFlagSet("desktop", flag.ContinueOnError)
	action := flags.String("action", "open", "open, stop or uninstall")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected desktop arguments")
	}
	work := func() error {
		m, err := installation.New()
		if err != nil {
			return err
		}
		ctx := context.Background()
		if *action == "stop" || *action == "uninstall" {
			text := "Stop Harbor and its ships? Closing a browser window keeps them running."
			if *action == "uninstall" {
				text = "Uninstall Harbor from this computer? Package-installed ships for all users will stop. Ship data, exports and configuration will be retained."
			}
			if err = exec.Command("zenity", "--question", "--title=Harbor", "--text="+text, "--ok-label=Continue", "--cancel-label=Cancel").Run(); err != nil {
				return nil
			}
			if *action == "stop" {
				return m.Stop(ctx)
			}
			if m.Source == "/usr/lib/harbor/harbor" {
				return exec.Command("pkexec", "/usr/bin/dpkg", "--remove", "harbor").Run()
			}
			if m.Source == "/usr/local/lib/harbor/harbor" {
				return exec.Command("pkexec", "/bin/sh", "/usr/local/lib/harbor/system-install.sh", "--uninstall").Run()
			}
			return m.Uninstall(ctx, "")
		}
		if *action != "open" {
			return errors.New("unknown desktop action")
		}
		progress := exec.Command("zenity", "--progress", "--pulsate", "--no-cancel", "--auto-close", "--title=Harbor", "--text=Starting Harbor…")
		input, pipeErr := progress.StdinPipe()
		closeProgress := func() {}
		if pipeErr == nil && progress.Start() == nil {
			closeProgress = func() { fmt.Fprintln(input, "100"); input.Close(); progress.Wait() }
		}
		defer func() { closeProgress() }()
		if err = m.Install(ctx, installation.Options{Upgrade: true, Headless: runtime.GOOS == "linux"}); err != nil {
			return err
		}
		if err = m.Start(ctx); err != nil {
			return err
		}
		status, err := m.Status(ctx)
		if err != nil {
			return err
		}
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		closeProgress()
		closeProgress = func() {}
		return exec.Command(opener, status.URL).Run()
	}
	if err := work(); err != nil {
		if runtime.GOOS == "linux" {
			exec.Command("zenity", "--error", "--title=Harbor", "--text="+err.Error()).Run()
		}
		return err
	}
	return nil
}

// packageCommand runs as root during system installation changes, but performs every
// user installation action as that user. It only touches records whose source
// is this exact package, and never deletes data or changes user lingering.
func packageCommand(args []string) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || len(args) != 1 || (args[0] != "upgrade" && args[0] != "remove") {
		return errors.New("package maintenance requires the Linux system installer")
	}
	contents, err := exec.Command("getent", "passwd").Output()
	if err != nil {
		return err
	}
	source, err := os.Executable()
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	return maintainPackageUsers(contents, args[0], source)
}

func packageRecord(home string) string {
	return filepath.Join(home, ".local", "share", "Harbor", "runner", "installation.json")
}
