package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"harbor/internal/app"
	"harbor/internal/config"
	"harbor/internal/jobs"
	"harbor/internal/onboarding"
	"harbor/internal/pier"
	"harbor/internal/scheduler"
	"harbor/internal/server"
	"harbor/internal/storage"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "harbor:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			return installCommand(args[1:])
		case "service":
			return serviceCommand(args[1:])
		case "desktop":
			return desktopCommand(args[1:])
		case "package-maintenance":
			return packageCommand(args[1:])
		}
	}
	if len(args) > 0 && args[0] == "retention" {
		return retain(args[1:])
	}
	if len(args) > 0 && args[0] == "version" {
		fmt.Println("Harbor", version)
		return nil
	}
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: harbor install [--headless] [--upgrade] | desktop | service start/stop/status/uninstall | serve [--config path] | retention [--config path] [--apply] | version")
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	path := flags.String("config", "", "JSON configuration path")
	if e := flags.Parse(args[1:]); e != nil {
		return e
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected arguments")
	}
	cfg, e := config.Load(*path)
	if e != nil {
		return e
	}
	storageCtx, cancelStorage := context.WithTimeout(context.Background(), 10*time.Second)
	storageGuard, e := storage.Acquire(storageCtx, cfg.RequiredVolumes, append([]string{cfg.DataDir}, cfg.AllowedRoots...))
	cancelStorage()
	if e != nil {
		return e
	}
	defer storageGuard.Close()
	if e = os.MkdirAll(cfg.DataDir, 0700); e != nil {
		return e
	}
	for _, dir := range cfg.AllowedRoots {
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
	}
	release, e := pier.Guard(cfg.DataDir)
	if e != nil {
		return errors.New("another Harbor instance is using this data directory")
	}
	defer release()
	st, e := store.Open(filepath.Join(cfg.DataDir, "harbor.sqlite"))
	if e != nil {
		return e
	}
	defer st.Close()
	listener, e := configuredListener(&cfg)
	if e != nil {
		return fmt.Errorf("listen %s: %w", cfg.ListenAddress, e)
	}
	defer listener.Close()
	sup := supervisor.New(st, nil)
	executor := jobs.New(st, sup)
	executor.EnableBackups(filepath.Join(cfg.DataDir, "backups"))
	executor.EnableReplay()
	executor.EnableRuntimeUpdates(filepath.Join(cfg.DataDir, "runtimes"))
	boot := &onboarding.Service{Config: cfg, Store: st, Executor: executor, Supervisor: sup, Uploads: onboarding.Uploads{Dir: filepath.Join(cfg.DataDir, "uploads")}}
	boot.Wire()
	protectedUploads, e := st.ProtectedUploads(context.Background())
	if e != nil {
		return e
	}
	if e = boot.Uploads.CleanupReferenced(time.Now(), protectedUploads); e != nil {
		return e
	}
	if e = app.RecoverAndStart(context.Background(), st, sup, executor); e != nil {
		return e
	}
	handler := server.New(cfg, st, executor)
	handler.SetVersion(version)
	handler.AttachRuntime(sup, boot)
	executor.OnChange = handler.Events.Publish
	sup.OnLog = handler.Events.Publish
	sup.OnChange = handler.Events.Publish
	svc := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerDone := make(chan error, 1)
	go func() { workerDone <- executor.Run(ctx) }()
	schedulerDone := make(chan error, 1)
	go func() { schedulerDone <- (scheduler.Worker{Store: st}).Run(ctx) }()
	done := make(chan error, 1)
	go func() { done <- svc.Serve(listener) }()
	// Keep an explicitly configured hostname in the URL; Host validation
	// admits that name, while the listener's resolved IP may differ.
	displayAddress := cfg.ListenAddress
	if host, port, err := net.SplitHostPort(displayAddress); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			displayAddress = net.JoinHostPort("127.0.0.1", port)
		}
	}
	fmt.Printf("Harbor %s · http://%s\n", version, displayAddress)

	workerFinished, schedulerFinished := false, false
	select {
	case e = <-done:
		if errors.Is(e, http.ErrServerClosed) {
			e = nil
		}
	case e = <-workerDone:
		workerFinished = true
	case e = <-schedulerDone:
		schedulerFinished = true
	case <-ctx.Done():
		e = nil
	}
	executor.SetReady(false)
	stop()
	// A utility is allowed to finish; no forced kill or database close races it.
	if !workerFinished {
		e = errors.Join(e, <-workerDone)
	}
	if !schedulerFinished {
		e = errors.Join(e, <-schedulerDone)
	}
	handler.Events.Close()
	// Keep the controller, PTY and database alive until owned runtimes exit.
	// Closing them at a fixed deadline can hang up an initializing ship before
	// it has handled a graceful stop. Like utilities above, native shutdown is
	// allowed to finish; it is never converted into a forced child termination.
	e = errors.Join(e, sup.Shutdown(context.Background()))
	shutdown, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	return errors.Join(e, svc.Shutdown(shutdown))
}
