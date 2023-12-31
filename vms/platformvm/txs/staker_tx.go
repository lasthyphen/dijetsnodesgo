// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto/bls"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/fx"
)

// ValidatorTx defines the interface for a validator transaction that supports
// delegation.
type ValidatorTx interface {
	UnsignedTx
	Validator
}

type DelegatorTx interface {
	UnsignedTx
	Delegator
}

type StakerTx interface {
	UnsignedTx
	Staker
}

type Validator interface {
	PermissionlessStaker

	ValidationRewardsOwner() fx.Owner
	DelegationRewardsOwner() fx.Owner
	Shares() uint32
}

type Delegator interface {
	PermissionlessStaker

	RewardsOwner() fx.Owner
}

type PermissionlessStaker interface {
	Staker

	Outputs() []*djtx.TransferableOutput
	Stake() []*djtx.TransferableOutput
}

type Staker interface {
	SubnetID() ids.ID
	NodeID() ids.NodeID
	// PublicKey returns the BLS public key registered by this transaction. If
	// there was no key registered by this transaction, it will return false.
	PublicKey() (*bls.PublicKey, bool, error)
	StartTime() time.Time
	EndTime() time.Time
	Weight() uint64
	PendingPriority() Priority
	CurrentPriority() Priority
}
