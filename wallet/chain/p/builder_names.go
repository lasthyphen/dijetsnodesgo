package p

import (
	"errors"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"
)

var errNoTargetKey = errors.New("keychain does not hold the target's key")

// NameUpdate is the new content of a name registration.
type NameUpdate struct {
	Target ids.ShortID
	Owner  *secp256k1fx.OutputOwners
	Expiry uint64
	// Revoke permanently retires this generation of the name. Target, Owner
	// and Expiry must then equal the current record's.
	Revoke bool
}

func (b *builder) NewRegisterNameTx(
	name string,
	target ids.ShortID,
	owner *secp256k1fx.OutputOwners,
	expiry uint64,
	registerFee uint64,
	options ...common.Option,
) (*txs.RegisterNameTx, error) {
	ops := common.NewOptions(options)
	targetAuth, err := b.authorizeNameTarget(target, ops)
	if err != nil {
		return nil, err
	}

	toBurn := map[ids.ID]uint64{
		b.backend.DJTXAssetID(): registerFee,
	}
	toStake := map[ids.ID]uint64{}
	inputs, outputs, _, err := b.spend(toBurn, toStake, ops)
	if err != nil {
		return nil, err
	}

	utils.Sort(owner.Addrs)
	return &txs.RegisterNameTx{
		BaseTx: txs.BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    b.backend.NetworkID(),
			BlockchainID: constants.PlatformChainID,
			Ins:          inputs,
			Outs:         outputs,
			Memo:         ops.Memo(),
		}},
		Name:       name,
		Target:     target,
		Owner:      owner,
		Expiry:     expiry,
		TargetAuth: targetAuth,
	}, nil
}

func (b *builder) NewUpdateNameTx(
	current *names.Record,
	update NameUpdate,
	options ...common.Option,
) (*txs.UpdateNameTx, error) {
	ops := common.NewOptions(options)

	currentOwner, ok := current.Owner.(*secp256k1fx.OutputOwners)
	if !ok {
		return nil, errUnknownOwnerType
	}
	nameSigIndices, ok := common.MatchOwners(currentOwner, ops.Addresses(b.addrs), ops.MinIssuanceTime())
	if !ok {
		return nil, errInsufficientAuthorization
	}

	targetAuth := &secp256k1fx.Input{}
	if update.Target != current.Target {
		var err error
		targetAuth, err = b.authorizeNameTarget(update.Target, ops)
		if err != nil {
			return nil, err
		}
	}

	toBurn := map[ids.ID]uint64{
		b.backend.DJTXAssetID(): b.backend.BaseTxFee(),
	}
	toStake := map[ids.ID]uint64{}
	inputs, outputs, _, err := b.spend(toBurn, toStake, ops)
	if err != nil {
		return nil, err
	}

	utils.Sort(update.Owner.Addrs)
	return &txs.UpdateNameTx{
		BaseTx: txs.BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    b.backend.NetworkID(),
			BlockchainID: constants.PlatformChainID,
			Ins:          inputs,
			Outs:         outputs,
			Memo:         ops.Memo(),
		}},
		Name:       current.Name,
		Version:    current.Version,
		Target:     update.Target,
		Owner:      update.Owner,
		Expiry:     update.Expiry,
		Revoke:     update.Revoke,
		NameAuth:   &secp256k1fx.Input{SigIndices: nameSigIndices},
		TargetAuth: targetAuth,
	}, nil
}

// authorizeNameTarget returns the input that proves the holder of [target]
// consents to a name pointing at it.
func (b *builder) authorizeNameTarget(target ids.ShortID, ops *common.Options) (*secp256k1fx.Input, error) {
	addrs := ops.Addresses(b.addrs)
	if !addrs.Contains(target) {
		return nil, errNoTargetKey
	}
	return &secp256k1fx.Input{SigIndices: []uint32{0}}, nil
}
