package txs

import (
	"errors"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/verify"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/fx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
)

var (
	_ UnsignedTx = (*RegisterNameTx)(nil)

	errEmptyNameTarget = errors.New("name target can't be empty")
	errZeroNameExpiry  = errors.New("name expiry can't be zero")
)

// RegisterNameTx registers a human-readable name in the native naming
// registry and points it at a V-chain identity.
type RegisterNameTx struct {
	// Metadata, inputs and outputs
	BaseTx `serialize:"true"`
	// Canonical name to register
	Name string `serialize:"true" json:"name"`
	// Identity that payments to the name are sent to
	Target ids.ShortID `serialize:"true" json:"target"`
	// Who is authorized to update the registration
	Owner fx.Owner `serialize:"true" json:"owner"`
	// Unix time, in M-chain time, at which the registration expires
	Expiry uint64 `serialize:"true" json:"expiry"`
	// Proves that the holder of [Target] consents to the name pointing at it.
	// Verified against the last credential.
	TargetAuth verify.Verifiable `serialize:"true" json:"targetAuthorization"`
}

func (tx *RegisterNameTx) InitCtx(ctx *snow.Context) {
	tx.BaseTx.InitCtx(ctx)
	tx.Owner.InitCtx(ctx)
}

func (tx *RegisterNameTx) SyntacticVerify(ctx *snow.Context) error {
	switch {
	case tx == nil:
		return ErrNilTx
	case tx.SyntacticallyVerified: // already passed syntactic verification
		return nil
	case tx.Target == ids.ShortEmpty:
		return errEmptyNameTarget
	case tx.Expiry == 0:
		return errZeroNameExpiry
	}
	if err := names.Validate(tx.Name); err != nil {
		return err
	}
	if err := tx.BaseTx.SyntacticVerify(ctx); err != nil {
		return err
	}
	if err := verify.All(tx.Owner, tx.TargetAuth); err != nil {
		return err
	}

	tx.SyntacticallyVerified = true
	return nil
}

func (tx *RegisterNameTx) Visit(visitor Visitor) error {
	return visitor.RegisterNameTx(tx)
}
