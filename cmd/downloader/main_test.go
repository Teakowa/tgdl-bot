package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"tgdl-bot/internal/config"
)

type fakePreflight struct {
	err error
}

func (f fakePreflight) Check(context.Context, config.Config) error {
	return f.err
}

type fakeLoop struct {
	called bool
}

func (l *fakeLoop) Run(context.Context, config.Config) error {
	l.called = true
	return nil
}

func TestRun_PreflightFailureSkipsLoop(t *testing.T) {
	cfg := config.Config{}
	logger := slog.Default()
	loop := &fakeLoop{}

	err := run(context.Background(), cfg, logger, fakePreflight{err: errors.New("preflight failed")}, loop)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if loop.called {
		t.Fatal("loop should not be called when preflight fails")
	}
}
