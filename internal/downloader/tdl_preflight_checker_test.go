package downloader

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeProbeExecutor struct {
	out      string
	err      error
	binary   string
	argsSeen []string
}

func (f *fakeProbeExecutor) Run(_ context.Context, binary string, args ...string) (string, error) {
	f.binary = binary
	f.argsSeen = append([]string(nil), args...)
	return f.out, f.err
}

func TestTDLPreflightChecker_CheckSuccess(t *testing.T) {
	exec := &fakeProbeExecutor{out: "[]"}
	checker := TDLPreflightChecker{Executor: exec}

	err := checker.Check(context.Background(), DownloadRequest{
		Binary:    "tdl",
		Namespace: "default",
		Storage:   "type=bolt,path=/root/.tdl/data",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.binary != "tdl" {
		t.Fatalf("expected tdl binary, got %q", exec.binary)
	}
	got := strings.Join(exec.argsSeen, " ")
	if !strings.Contains(got, "-n default") {
		t.Fatalf("missing namespace arg: %q", got)
	}
	if !strings.Contains(got, "--storage type=bolt,path=/root/.tdl/data") {
		t.Fatalf("missing storage arg: %q", got)
	}
	if !strings.Contains(got, "chat ls -o json -f false") {
		t.Fatalf("missing chat ls probe args: %q", got)
	}
}

func TestTDLPreflightChecker_CheckFailureIncludesOutput(t *testing.T) {
	exec := &fakeProbeExecutor{
		out: "authorization required",
		err: errors.New("exit status 1"),
	}
	checker := TDLPreflightChecker{Executor: exec}
	err := checker.Check(context.Background(), DownloadRequest{
		Binary:    "tdl",
		Namespace: "default",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "tdl preflight probe failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "authorization required") {
		t.Fatalf("expected probe output in error, got: %v", err)
	}
}

func TestTDLPreflightChecker_RequiresBinaryAndNamespace(t *testing.T) {
	checker := TDLPreflightChecker{}
	if err := checker.Check(context.Background(), DownloadRequest{Namespace: "default"}); err == nil {
		t.Fatal("expected empty binary error")
	}
	if err := checker.Check(context.Background(), DownloadRequest{Binary: "tdl"}); err == nil {
		t.Fatal("expected empty namespace error")
	}
}
