package downloader

import (
	"context"
	"errors"
	"net"
)

var (
	ErrRetryable    = errors.New("downloader: retryable")
	ErrNonRetryable = errors.New("downloader: non-retryable")
)

type ErrorClass string

const (
	ErrorClassUnknown      ErrorClass = "unknown"
	ErrorClassRetryable    ErrorClass = "retryable"
	ErrorClassNonRetryable ErrorClass = "non_retryable"
)

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassUnknown
	}
	if errors.Is(err, ErrNonRetryable) {
		return ErrorClassNonRetryable
	}
	if errors.Is(err, ErrRetryable) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassRetryable
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return ErrorClassRetryable
	}

	return ErrorClassUnknown
}

func IsRetryableError(err error) bool {
	return ClassifyError(err) == ErrorClassRetryable
}
