package downloader

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
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
	URL               string
	DownloadDir       string
	Binary            string
	Namespace         string
	Storage           string
	Group             bool
	SkipSame          bool
	TaskConcurrency   int
	ThreadConcurrency int
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
	return buildDownloadCommand(ctx, req)
}

type commandBuilder struct{}

func NewCommandBuilder() CommandBuilder {
	return commandBuilder{}
}

func (commandBuilder) BuildCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error) {
	return buildDownloadCommand(ctx, req)
}

func buildDownloadCommand(ctx context.Context, req DownloadRequest) (*exec.Cmd, error) {
	args, err := buildDownloadArgs(req)
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

func buildDownloadArgs(req DownloadRequest) ([]string, error) {
	if req.URL == "" {
		return nil, errors.New("downloader: URL is required")
	}
	if req.DownloadDir == "" {
		return nil, errors.New("downloader: download directory is required")
	}

	taskConcurrency := req.TaskConcurrency
	if taskConcurrency <= 0 {
		taskConcurrency = 1
	}

	threadConcurrency := req.ThreadConcurrency
	if threadConcurrency <= 0 {
		threadConcurrency = 4
	}

	args := []string{
		"dl",
		"-u", req.URL,
		"-d", req.DownloadDir,
	}

	if req.Namespace != "" {
		args = append(args, "--namespace", req.Namespace)
	}
	if req.Storage != "" {
		args = append(args, "--storage", req.Storage)
	}
	if req.Group {
		args = append(args, "--group")
	}
	if req.SkipSame {
		args = append(args, "--skip-same")
	}

	args = append(args,
		"-l", strconv.Itoa(taskConcurrency),
		"-t", strconv.Itoa(threadConcurrency),
	)

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
