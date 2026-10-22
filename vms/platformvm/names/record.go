package names

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/fx"
)

// Status is the lifecycle state of a name. It is derived from a record and
// a chain timestamp; it is never stored.
type Status string

const (
	StatusNotFound Status = "NOT_FOUND"
	StatusActive   Status = "ACTIVE"
	StatusExpired  Status = "EXPIRED"
	StatusRevoked  Status = "REVOKED"
	StatusReleased Status = "RELEASED"
)

// Record is one version of a name registration as stored in M-chain state.
type Record struct {
	Name   string      `serialize:"true" json:"name"`
	Target ids.ShortID `serialize:"true" json:"target"`
	Owner  fx.Owner    `serialize:"true" json:"owner"`
	// Expiry is in Unix seconds of M-chain time.
	Expiry  uint64 `serialize:"true" json:"expiry"`
	Revoked bool   `serialize:"true" json:"revoked"`
	// Generation is 1 for the first registration of a name and increases
	// each time a released name is registered again.
	Generation uint64 `serialize:"true" json:"generation"`
	// Version is 1 at registration and increases with every accepted update
	// within a generation.
	Version uint64 `serialize:"true" json:"version"`
	// RecordID is the ID of the RegisterNameTx that started this generation.
	RecordID ids.ID `serialize:"true" json:"recordID"`
	// LastTxID is the ID of the transaction that produced this version.
	LastTxID     ids.ID `serialize:"true" json:"lastTxID"`
	RegisteredAt uint64 `serialize:"true" json:"registeredAt"`
	UpdatedAt    uint64 `serialize:"true" json:"updatedAt"`
}

// StatusAt returns the status of [r] at chain time [now]. A nil record is
// NOT_FOUND.
func (r *Record) StatusAt(now time.Time, grace time.Duration) Status {
	if r == nil {
		return StatusNotFound
	}
	t := uint64(now.Unix())
	g := uint64(grace / time.Second)
	switch {
	case r.Revoked && t < r.UpdatedAt+g:
		return StatusRevoked
	case r.Revoked:
		return StatusReleased
	case t < r.Expiry:
		return StatusActive
	case t < r.Expiry+g:
		return StatusExpired
	default:
		return StatusReleased
	}
}

// Resolvable reports whether payments may be directed to the record's target.
func (s Status) Resolvable() bool {
	return s == StatusActive
}

// Updatable reports whether the record's owner may still change it.
func (s Status) Updatable() bool {
	return s == StatusActive || s == StatusExpired
}

// Registrable reports whether anyone may register the name.
func (s Status) Registrable() bool {
	return s == StatusNotFound || s == StatusReleased
}

// HoldsTarget reports whether a record in this status still reserves its
// target address, so that no other name may point at it.
func (s Status) HoldsTarget() bool {
	return s != StatusNotFound && s != StatusReleased
}
