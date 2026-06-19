package repository

import "errors"

// ErrNotFound is returned when a requested document does not exist.
var ErrNotFound = errors.New("not found")
