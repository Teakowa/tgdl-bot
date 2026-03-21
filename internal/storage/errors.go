package storage

import "errors"

var (
	ErrNotImplemented = errors.New("storage: not implemented")
	ErrTaskNotFound   = errors.New("storage: task not found")
)
