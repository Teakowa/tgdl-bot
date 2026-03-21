package downloader

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

type fakeRunner struct {
	state SessionState
	err   error
}

func (r fakeRunner) Preflight(context.Context, DownloadRequest) (SessionState, error) {
	return r.state, r.err
}

func (r fakeRunner) BuildCommand(context.Context, DownloadRequest) (*exec.Cmd, error) {
	return nil, nil
}

func TestStartupPreflightRequiresReadyWhenLoginRequired(t *testing.T) {
	p := StartupPreflight{
		Runner: fakeRunner{state: SessionStateInvalid},
	}
	err := p.Check(context.Background(), StartupConfig{
		Binary:        "sh",
		Namespace:     "default",
		LoginRequired: true,
	})
	if err == nil {
		t.Fatal("expected non-ready session to fail when login is required")
	}
}

func TestStartupPreflightPassesWithReadySession(t *testing.T) {
	p := StartupPreflight{
		Runner: fakeRunner{state: SessionStateReady},
	}
	err := p.Check(context.Background(), StartupConfig{
		Binary:        "sh",
		Namespace:     "default",
		LoginRequired: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartupPreflightPropagatesRunnerError(t *testing.T) {
	p := StartupPreflight{
		Runner: fakeRunner{err: errors.New("session probe failed")},
	}
	err := p.Check(context.Background(), StartupConfig{
		Binary:        "sh",
		Namespace:     "default",
		LoginRequired: false,
	})
	if err == nil {
		t.Fatal("expected runner error to be returned")
	}
}
