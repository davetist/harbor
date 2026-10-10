package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"harbor/internal/config"
	"harbor/internal/pier"
	"harbor/internal/retention"
	"harbor/internal/storage"
	"harbor/internal/store"
	"os"
	"path/filepath"
	"time"
)

func retain(args []string) error {
	flags := flag.NewFlagSet("retention", flag.ContinueOnError)
	path := flags.String("config", "", "Harbor configuration")
	uploadDays := flags.Int("upload-days", 0, "prune unreferenced archive uploads older than this age; 0 keeps all")
	exportDays := flags.Int("export-days", 0, "prune old exports; 0 keeps all")
	keep := flags.Int("keep-exports", 2, "minimum newest verified exports retained per ship (at least 2)")
	budget := flags.Int64("budget-bytes", 0, "report budget; refuse apply if retained files exceed it (0 disables)")
	apply := flags.Bool("apply", false, "remove only eligible files in this report; requires stopped Harbor")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("unexpected retention arguments")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	guard, err := storage.Acquire(ctx, cfg.RequiredVolumes, append([]string{cfg.DataDir}, cfg.AllowedRoots...))
	if err != nil {
		return err
	}
	defer guard.Close()
	db := filepath.Join(cfg.DataDir, "harbor.sqlite")
	if info, err := os.Lstat(db); err != nil || !info.Mode().IsRegular() {
		return errors.New("existing Harbor database required")
	}
	release, err := pier.Guard(cfg.DataDir)
	if err != nil {
		return errors.New("stop Harbor before running retention so jobs and downloads cannot race cleanup")
	}
	defer release()
	st, err := store.Open(db)
	if err != nil {
		return err
	}
	defer st.Close()
	report, err := retention.Plan(ctx, st, cfg.DataDir, retention.Policy{UploadDays: *uploadDays, ExportDays: *exportDays, KeepExports: *keep, BudgetBytes: *budget}, time.Now())
	if err != nil {
		return err
	}
	if *apply {
		err = retention.Apply(ctx, st, &report)
	}
	output := json.NewEncoder(os.Stdout)
	output.SetIndent("", "  ")
	return errors.Join(err, output.Encode(report))
}
