package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type StartupConfig struct {
	Binary        string
	DownloadDir   string
	Namespace     string
	Storage       string
	LoginRequired bool
}

type StartupPreflight struct {
	Runner Runner
}

func (p StartupPreflight) Check(ctx context.Context, cfg StartupConfig) error {
	if cfg.Binary == "" {
		return errors.New("downloader preflight: empty tdl binary")
	}
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		return fmt.Errorf("downloader preflight: tdl binary not found: %w", err)
	}
	if cfg.DownloadDir == "" {
		return errors.New("downloader preflight: empty download directory")
	}
	if err := ensureWritableDir(cfg.DownloadDir); err != nil {
		return fmt.Errorf("downloader preflight: download directory not writable: %w", err)
	}
	if cfg.Namespace == "" {
		return errors.New("downloader preflight: empty tdl namespace")
	}

	if p.Runner == nil {
		if cfg.LoginRequired {
			return errors.New("downloader preflight: runner required when login check is enabled")
		}
		return nil
	}

	state, err := p.Runner.Preflight(ctx, DownloadRequest{
		Binary:      cfg.Binary,
		DownloadDir: cfg.DownloadDir,
		Namespace:   cfg.Namespace,
		Storage:     cfg.Storage,
	})
	if err != nil {
		return fmt.Errorf("downloader preflight: session check failed: %w", err)
	}
	if err := ValidatePreflightState(state); err != nil {
		return err
	}
	if cfg.LoginRequired && state != SessionStateReady {
		return fmt.Errorf("downloader preflight: session is not ready (%s)", state)
	}
	return nil
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	probe := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	if err := os.Remove(probe); err != nil {
		return err
	}
	return nil
}
