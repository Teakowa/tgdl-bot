package downloader

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"time"
)

type SessionState string

const (
	SessionStateUnknown SessionState = "unknown"
	SessionStateReady   SessionState = "ready"
	SessionStateInvalid SessionState = "invalid"
	SessionStateExpired SessionState = "expired"
	SessionStateBlocked SessionState = "blocked"
)

type DownloadRequest struct {
	URL          string
	TargetChatID int64
	Binary       string
	Namespace    string
	Storage      string
}

type RunResult struct {
	Command  []string
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

type PreflightChecker interface {
	Check(ctx context.Context, req DownloadRequest) error
}

type CommandBuilder interface {
	BuildCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error)
}

type Runner interface {
	Preflight(ctx context.Context, req DownloadRequest) (SessionState, error)
	BuildCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error)
}

type DefaultRunner struct {
	PreflightChecker PreflightChecker
	CommandBuilder   CommandBuilder
}

func (r DefaultRunner) Preflight(ctx context.Context, req DownloadRequest) (SessionState, error) {
	if r.PreflightChecker == nil {
		return SessionStateUnknown, nil
	}
	if err := r.PreflightChecker.Check(ctx, req); err != nil {
		return SessionStateInvalid, err
	}
	return SessionStateReady, nil
}

func (r DefaultRunner) BuildCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error) {
	if r.CommandBuilder != nil {
		return r.CommandBuilder.BuildCommand(ctx, req)
	}
	return buildForwardCommand(ctx, req)
}

type commandBuilder struct{}

func NewCommandBuilder() CommandBuilder {
	return commandBuilder{}
}

func (commandBuilder) BuildCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error) {
	return buildForwardCommand(ctx, req)
}

func buildForwardCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error) {
	args, err := buildForwardArgs(req)
	if err != nil {
		return nil, err
	}

	binary := req.Binary
	if binary == "" {
		binary = "tdl"
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	return cmd, nil
}

var telegramURLPartsPattern = regexp.MustCompile(`^https?://(?:t\.me|telegram\.me)/(?:c/\d+/|[A-Za-z0-9_]+/)(\d+)$`)

const defaultReconnectTimeout = "10m"

func buildForwardArgs(req DownloadRequest) ([]string, error) {
	if req.URL == "" {
		return nil, errors.New("downloader: URL is required")
	}

	matches := telegramURLPartsPattern.FindStringSubmatch(req.URL)
	if len(matches) != 2 || matches[1] == "" {
		return nil, errors.New("downloader: unsupported telegram message URL")
	}

	args := []string{
		"forward",
		"--from", req.URL,
	}
	if req.TargetChatID != 0 {
		args = append(args, "--to", fmt.Sprintf("%d", req.TargetChatID))
	}
	args = append(args, "--reconnect-timeout", defaultReconnectTimeout)

	if req.Namespace != "" {
		args = append(args, "--ns", req.Namespace)
	}
	if req.Storage != "" {
		args = append(args, "--storage", req.Storage)
	}

	return args, nil
}

func ValidatePreflightState(state SessionState) error {
	switch state {
	case SessionStateReady:
		return nil
	case SessionStateUnknown, SessionStateInvalid, SessionStateExpired, SessionStateBlocked:
		return nil
	default:
		return fmt.Errorf("downloader: unknown session state %q", state)
	}
}
