package repository

import "errors"

// ErrNotFound is returned when a requested document does not exist.
var ErrNotFound = errors.New("not found")

// ErrActiveSessionExists is returned when creating a session while another
// active (gathering/voting/reading) session already exists.
var ErrActiveSessionExists = errors.New("an active session already exists")
