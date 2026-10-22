package executor

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/verify"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/fx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/state"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/utxo"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

var (
	ErrNamingNotActivated         = errors.New("native naming is not activated")
	ErrNameNotRegistrable         = errors.New("name is not available for registration")
	ErrNameNotFound               = errors.New("name is not registered")
	ErrNameNotUpdatable           = errors.New("name can no longer be updated")
	ErrNameVersionMismatch        = errors.New("name record version mismatch")
	ErrNameTargetTaken            = errors.New("target is already bound to another name")
	ErrNameExpiryOutOfRange       = errors.New("name expiry out of range")
	ErrUnauthorizedNameUpdate     = errors.New("unauthorized name update")
	ErrUnauthorizedNameTarget     = errors.New("target did not authorize the name")
	ErrUnexpectedNameTargetAuth   = errors.New("target authorization must be empty when the target is unchanged")
	ErrRevokeChangesRecord        = errors.New("a revocation can't change the target, owner or expiry")
	errWrongNumberOfNameAuthCreds = errors.New("missing name authorization credentials")
)

func (e *StandardTxExecutor) RegisterNameTx(tx *txs.RegisterNameTx) error {
	if err := e.Tx.SyntacticVerify(e.Ctx); err != nil {
		return err
	}

	now := e.State.GetTimestamp()
	if !e.Config.IsNamingActivated(now) {
		return ErrNamingNotActivated
	}
	params := e.Config.NamingParams

	baseCreds, targetCred, err := splitLastCredential(e.Tx.Creds)
	if err != nil {
		return err
	}

	if err := verifyNameExpiry(now, tx.Expiry, params.MinTerm, params.MaxTerm); err != nil {
		return err
	}

	prev, err := getNameRecord(e.State, tx.Name)
	if err != nil {
		return err
	}
	if status := prev.StatusAt(now, params.GracePeriod); !status.Registrable() {
		return fmt.Errorf("%w: %q is %s", ErrNameNotRegistrable, tx.Name, status)
	}

	if err := verifyNameTargetFree(e.State, now, params.GracePeriod, tx.Target, tx.Name); err != nil {
		return err
	}
	if err := e.verifyNameTargetAuth(tx.TargetAuth, targetCred, tx.Target); err != nil {
		return err
	}

	// Verify the flowcheck
	if err := e.FlowChecker.VerifySpend(
		tx,
		e.State,
		tx.Ins,
		tx.Outs,
		baseCreds,
		map[ids.ID]uint64{
			e.Ctx.DJTXAssetID: params.RegisterFee,
		},
	); err != nil {
		return err
	}

	txID := e.Tx.ID()
	timestamp := uint64(now.Unix())
	generation := uint64(1)
	if prev != nil {
		generation = prev.Generation + 1
		// Drop the released generation's reverse entry unless another name
		// has claimed that target since.
		if err := deleteNameTargetIfBound(e.State, prev.Target, tx.Name); err != nil {
			return err
		}
	}

	// Consume the UTXOS
	utxo.Consume(e.State, tx.Ins)
	// Produce the UTXOS
	utxo.Produce(e.State, txID, tx.Outs)

	e.State.PutName(&names.Record{
		Name:         tx.Name,
		Target:       tx.Target,
		Owner:        tx.Owner,
		Expiry:       tx.Expiry,
		Generation:   generation,
		Version:      1,
		RecordID:     txID,
		LastTxID:     txID,
		RegisteredAt: timestamp,
		UpdatedAt:    timestamp,
	})
	e.State.PutNameTarget(tx.Target, tx.Name)
	return nil
}

func (e *StandardTxExecutor) UpdateNameTx(tx *txs.UpdateNameTx) error {
	if err := e.Tx.SyntacticVerify(e.Ctx); err != nil {
		return err
	}

	now := e.State.GetTimestamp()
	if !e.Config.IsNamingActivated(now) {
		return ErrNamingNotActivated
	}
	params := e.Config.NamingParams

	creds, targetCred, err := splitLastCredential(e.Tx.Creds)
	if err != nil {
		return err
	}
	baseCreds, nameCred, err := splitLastCredential(creds)
	if err != nil {
		return err
	}

	record, err := getNameRecord(e.State, tx.Name)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("%w: %q", ErrNameNotFound, tx.Name)
	}
	if status := record.StatusAt(now, params.GracePeriod); !status.Updatable() {
		return fmt.Errorf("%w: %q is %s", ErrNameNotUpdatable, tx.Name, status)
	}
	if tx.Version != record.Version {
		return fmt.Errorf("%w: update is for version %d but the record is at version %d",
			ErrNameVersionMismatch,
			tx.Version,
			record.Version,
		)
	}

	if err := e.Fx.VerifyPermission(tx, tx.NameAuth, nameCred, record.Owner); err != nil {
		return fmt.Errorf("%w: %s", ErrUnauthorizedNameUpdate, err)
	}

	targetChanged := tx.Target != record.Target
	if tx.Revoke {
		sameOwner, err := ownersEqual(tx.Owner, record.Owner)
		if err != nil {
			return err
		}
		if targetChanged || !sameOwner || tx.Expiry != record.Expiry {
			return ErrRevokeChangesRecord
		}
	} else if err := verifyNameExpiry(now, tx.Expiry, time.Second, params.MaxTerm); err != nil {
		return err
	}

	if targetChanged {
		if err := verifyNameTargetFree(e.State, now, params.GracePeriod, tx.Target, tx.Name); err != nil {
			return err
		}
		if err := e.verifyNameTargetAuth(tx.TargetAuth, targetCred, tx.Target); err != nil {
			return err
		}
	} else if !isEmptyAuth(tx.TargetAuth, targetCred) {
		// Unverified signature bytes would otherwise alter the tx ID.
		return ErrUnexpectedNameTargetAuth
	}

	// Verify the flowcheck
	if err := e.FlowChecker.VerifySpend(
		tx,
		e.State,
		tx.Ins,
		tx.Outs,
		baseCreds,
		map[ids.ID]uint64{
			e.Ctx.DJTXAssetID: e.Config.TxFee,
		},
	); err != nil {
		return err
	}

	txID := e.Tx.ID()

	// Consume the UTXOS
	utxo.Consume(e.State, tx.Ins)
	// Produce the UTXOS
	utxo.Produce(e.State, txID, tx.Outs)

	e.State.PutName(&names.Record{
		Name:         record.Name,
		Target:       tx.Target,
		Owner:        tx.Owner,
		Expiry:       tx.Expiry,
		Revoked:      tx.Revoke,
		Generation:   record.Generation,
		Version:      record.Version + 1,
		RecordID:     record.RecordID,
		LastTxID:     txID,
		RegisteredAt: record.RegisteredAt,
		UpdatedAt:    uint64(now.Unix()),
	})
	if targetChanged {
		if err := deleteNameTargetIfBound(e.State, record.Target, tx.Name); err != nil {
			return err
		}
		e.State.PutNameTarget(tx.Target, tx.Name)
	}
	return nil
}

func (e *StandardTxExecutor) verifyNameTargetAuth(
	auth verify.Verifiable,
	cred verify.Verifiable,
	target ids.ShortID,
) error {
	owner := &secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs:     []ids.ShortID{target},
	}
	if err := e.Fx.VerifyPermission(e.Tx.Unsigned, auth, cred, owner); err != nil {
		return fmt.Errorf("%w: %s", ErrUnauthorizedNameTarget, err)
	}
	return nil
}

// splitLastCredential separates the last credential, which authorizes a
// name operation, from the credentials of the base tx inputs.
func splitLastCredential(creds []verify.Verifiable) ([]verify.Verifiable, verify.Verifiable, error) {
	if len(creds) == 0 {
		return nil, nil, errWrongNumberOfNameAuthCreds
	}
	last := len(creds) - 1
	return creds[:last], creds[last], nil
}

// getNameRecord returns the record of [name], or nil if it has never been
// registered.
func getNameRecord(chainState state.Chain, name string) (*names.Record, error) {
	record, err := chainState.GetName(name)
	if err == database.ErrNotFound {
		return nil, nil
	}
	return record, err
}

// verifyNameTargetFree checks that no name other than [name] still holds
// [target].
func verifyNameTargetFree(
	chainState state.Chain,
	now time.Time,
	grace time.Duration,
	target ids.ShortID,
	name string,
) error {
	holder, err := chainState.GetNameTarget(target)
	if err == database.ErrNotFound || holder == name {
		return nil
	}
	if err != nil {
		return err
	}
	record, err := getNameRecord(chainState, holder)
	if err != nil {
		return err
	}
	if record != nil && record.Target == target && record.StatusAt(now, grace).HoldsTarget() {
		return fmt.Errorf("%w: %q", ErrNameTargetTaken, holder)
	}
	return nil
}

func deleteNameTargetIfBound(chainState state.Chain, target ids.ShortID, name string) error {
	holder, err := chainState.GetNameTarget(target)
	switch {
	case err == database.ErrNotFound:
		return nil
	case err != nil:
		return err
	case holder == name:
		chainState.DeleteNameTarget(target)
	}
	return nil
}

func verifyNameExpiry(now time.Time, expiry uint64, minTerm, maxTerm time.Duration) error {
	t := uint64(now.Unix())
	earliest := t + uint64(minTerm/time.Second)
	latest := t + uint64(maxTerm/time.Second)
	if expiry < earliest || expiry > latest {
		return fmt.Errorf("%w: %d is not within [%d, %d]", ErrNameExpiryOutOfRange, expiry, earliest, latest)
	}
	return nil
}

func isEmptyAuth(auth verify.Verifiable, cred verify.Verifiable) bool {
	in, ok := auth.(*secp256k1fx.Input)
	if !ok || len(in.SigIndices) != 0 {
		return false
	}
	c, ok := cred.(*secp256k1fx.Credential)
	return ok && len(c.Sigs) == 0
}

func ownersEqual(a, b fx.Owner) (bool, error) {
	aBytes, err := txs.Codec.Marshal(txs.Version, &a)
	if err != nil {
		return false, err
	}
	bBytes, err := txs.Codec.Marshal(txs.Version, &b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(aBytes, bBytes), nil
}
