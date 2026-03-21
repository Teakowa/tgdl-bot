package downloader

import (
	"context"
	"errors"
	"testing"
)

func TestBuildDownloadArgs(t *testing.T) {
	got, err := buildDownloadArgs(DownloadRequest{
		URL:               "https://t.me/c/123/456",
		DownloadDir:       "/tmp/downloads",
		Namespace:         "default",
		Storage:           "/tmp/tdl-storage",
		Group:             true,
		SkipSame:          true,
		TaskConcurrency:   1,
		ThreadConcurrency: 4,
	})
	if err != nil {
		t.Fatalf("buildDownloadArgs returned error: %v", err)
	}

	want := []string{
		"dl",
		"-u", "https://t.me/c/123/456",
		"-d", "/tmp/downloads",
		"--namespace", "default",
		"--storage", "/tmp/tdl-storage",
		"--group",
		"--skip-same",
		"-l", "1",
		"-t", "4",
	}

	if len(got) != len(want) {
		t.Fatalf("arg length mismatch: got %d want %d\n got=%v\nwant=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d mismatch: got %q want %q\n got=%v\nwant=%v", i, got[i], want[i], got, want)
		}
	}
}

func TestClassifyError(t *testing.T) {
	timeoutErr := &testNetError{timeout: true, temporary: true}
	if got := ClassifyError(timeoutErr); got != ErrorClassRetryable {
		t.Fatalf("expected retryable, got %s", got)
	}
	if !IsRetryableError(timeoutErr) {
		t.Fatal("expected timeout error to be retryable")
	}

	if got := ClassifyError(errors.Join(ErrNonRetryable, context.Canceled)); got != ErrorClassNonRetryable {
		t.Fatalf("expected non-retryable, got %s", got)
	}
}

type testNetError struct {
	timeout   bool
	temporary bool
}

func (e *testNetError) Error() string   { return "test network error" }
func (e *testNetError) Timeout() bool   { return e.timeout }
func (e *testNetError) Temporary() bool { return e.temporary }
