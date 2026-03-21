package downloader

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

type StartupConfig struct {
	Binary        string
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
		Binary:    cfg.Binary,
		Namespace: cfg.Namespace,
		Storage:   cfg.Storage,
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
