

package common

import (
	"net/http"
)

// LockOption allows the vm to specify their lock option based on their endpoint
type LockOption uint32

// List of all allowed options
const (
	WriteLock = iota
	ReadLock
	NoLock
)

type HTTPHandler struct {
	LockOptions LockOption
	Handler     http.Handler
}
