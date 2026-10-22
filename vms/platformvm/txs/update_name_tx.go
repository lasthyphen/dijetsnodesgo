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
	_ UnsignedTx = (*UpdateNameTx)(nil)

	errZeroNameVersion = errors.New("name version can't be zero")
)

// UpdateNameTx replaces the contents of a name registration. It covers
// changing the target, transferring ownership, renewing and revoking.
//
// The update applies only if [Version] equals the record's current version,
// so a stale or concurrent update fails instead of silently overwriting.
type UpdateNameTx struct {
	// Metadata, inputs and outputs
	BaseTx `serialize:"true"`
	// Canonical name to update
	Name string `serialize:"true" json:"name"`
	// Version of the record this update replaces
	Version uint64 `serialize:"true" json:"version"`
	// New identity that payments to the name are sent to
	Target ids.ShortID `serialize:"true" json:"target"`
	// New owner of the registration
	Owner fx.Owner `serialize:"true" json:"owner"`
	// New expiry, in Unix seconds of M-chain time
	Expiry uint64 `serialize:"true" json:"expiry"`
	// Permanently retires this generation of the name
	Revoke bool `serialize:"true" json:"revoke"`
	// Proves that the issuer is the record's current owner. Verified against
	// the second-to-last credential.
	NameAuth verify.Verifiable `serialize:"true" json:"nameAuthorization"`
	// Proves that the holder of a new [Target] consents to the name pointing
	// at it. Verified against the last credential. Must be empty, with an
	// empty credential, when the target is unchanged.
	TargetAuth verify.Verifiable `serialize:"true" json:"targetAuthorization"`
}

func (tx *UpdateNameTx) InitCtx(ctx *snow.Context) {
	tx.BaseTx.InitCtx(ctx)
	tx.Owner.InitCtx(ctx)
}

func (tx *UpdateNameTx) SyntacticVerify(ctx *snow.Context) error {
	switch {
	case tx == nil:
		return ErrNilTx
	case tx.SyntacticallyVerified: // already passed syntactic verification
		return nil
	case tx.Version == 0:
		return errZeroNameVersion
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
	if err := verify.All(tx.Owner, tx.NameAuth, tx.TargetAuth); err != nil {
		return err
	}

	tx.SyntacticallyVerified = true
	return nil
}

func (tx *UpdateNameTx) Visit(visitor Visitor) error {
	return visitor.UpdateNameTx(tx)
}
