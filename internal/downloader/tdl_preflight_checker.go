package downloader

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type ProbeExecutor interface {
	Run(ctx context.Context, binary string, args ...string) (string, error)
}

type defaultProbeExecutor struct{}

func (defaultProbeExecutor) Run(ctx context.Context, binary string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type TDLPreflightChecker struct {
	Executor ProbeExecutor
}

func NewTDLPreflightChecker() TDLPreflightChecker {
	return TDLPreflightChecker{Executor: defaultProbeExecutor{}}
}

func (c TDLPreflightChecker) Check(ctx context.Context, req DownloadRequest) error {
	if req.Binary == "" {
		return fmt.Errorf("tdl preflight probe: empty binary")
	}
	if req.Namespace == "" {
		return fmt.Errorf("tdl preflight probe: empty namespace")
	}

	executor := c.Executor
	if executor == nil {
		executor = defaultProbeExecutor{}
	}

	args := []string{"-n", req.Namespace}
	if req.Storage != "" {
		args = append(args, "--storage", req.Storage)
	}
	// Read-only probe that requires valid login session but avoids large payload.
	args = append(args, "chat", "ls", "-o", "json", "-f", "false")

	out, err := executor.Run(ctx, req.Binary, args...)
	if err != nil {
		trimmed := truncateForLog(out, 240)
		if trimmed != "" {
			return fmt.Errorf("tdl preflight probe failed: %w: %s", err, trimmed)
		}
		return fmt.Errorf("tdl preflight probe failed: %w", err)
	}
	return nil
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
