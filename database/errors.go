

package database

import "errors"

// common errors
var (
	ErrClosed   = errors.New("closed")
	ErrNotFound = errors.New("not found")
)
