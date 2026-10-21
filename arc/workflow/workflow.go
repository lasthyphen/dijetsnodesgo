// Package workflow implements the client-side Arc creation workflow.
//
// Every state change is an ordinary, signed M-chain transaction issued by the
// caller's wallet and decided by M-chain consensus:
//
//  1. CreateSubnetTx       (skipped when Request.SubnetID is set)
//  2. AddSubnetValidatorTx for every committee member not yet on the subnet
//  3. wait until every committee member is a current subnet validator
//  4. CreateChainTx
//
// The workflow adds the checks the protocol does not perform (VM-aware
// genesis validation, naming policy, committee eligibility) and makes retries
// idempotent: re-running a request against the same subnet never creates a
// duplicate chain or a duplicate committee entry.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/arc"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/fxs"
	"github.com/lasthyphen/dijetsnodesgo/vms/nftfx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/validator"
	"github.com/lasthyphen/dijetsnodesgo/vms/propertyfx"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/p"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"

	avmtxs "github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
)

var (
	ErrGenesisNoAssets       = errors.New("genesis must define at least one asset (the first one is the fee asset)")
	ErrGenesisAssetHasOuts   = errors.New("genesis assets must not have outputs")
	ErrCommitteeTooSmall     = errors.New("committee is smaller than the minimum committee size")
	ErrDuplicateMember       = errors.New("duplicate committee member")
	ErrZeroWeight            = errors.New("committee member weight must be positive")
	ErrNotPrimaryValidator   = errors.New("committee member is not a current primary network validator")
	ErrPeriodNotBounded      = errors.New("validation period must be within the member's primary network validation period")
	ErrInvalidPeriod         = errors.New("validation end must be after its start")
	ErrMissingOwner          = errors.New("owner is required when creating a new subnet")
	ErrConflictingRegistered = errors.New("a chain with this name is already registered on the subnet with a different genesis or VM")
)

// Member is a committee member to assign.
type Member struct {
	NodeID ids.NodeID
	Weight uint64
}

// Request describes an Arc to create.
type Request struct {
	Name    string
	Genesis []byte
	// SubnetID reuses an existing subnet owned by the wallet's keys. When
	// empty, a new subnet owned by Owner is created.
	SubnetID ids.ID
	Owner    *secp256k1fx.OutputOwners
	// Committee members must be current primary network validators whose
	// primary validation period contains [ValidationStart, ValidationEnd].
	Committee        []Member
	ValidationStart  time.Time
	ValidationEnd    time.Time
	MinCommitteeSize int
}

// Result reports what the workflow did. Persist it: passing SubnetID back in a
// retried Request makes the retry idempotent.
type Result struct {
	SubnetID       ids.ID
	ChainID        ids.ID
	ValidatorTxIDs []ids.ID
	CreatedSubnet  bool
	CreatedChain   bool
}

// FxIDs are the feature extensions every Arc AVM runs with.
var FxIDs = []ids.ID{secp256k1fx.ID}

func newAVMParser() (avmtxs.Parser, error) {
	return avmtxs.NewParser([]fxs.Fx{
		&secp256k1fx.Fx{},
		&nftfx.Fx{},
		&propertyfx.Fx{},
	})
}

// BuildAVMGenesis builds AVM genesis bytes from asset definitions, using the
// same code path as the avm.buildGenesis static API. Memos are 0x-prefixed
// hex without checksum. The first asset by alias is the chain's fee asset.
func BuildAVMGenesis(networkID uint32, assets map[string]avm.AssetDefinition) ([]byte, error) {
	reply := avm.BuildGenesisReply{}
	err := avm.CreateStaticService().BuildGenesis(nil, &avm.BuildGenesisArgs{
		NetworkID:   json.Uint32(networkID),
		GenesisData: assets,
		Encoding:    formatting.HexNC,
	}, &reply)
	if err != nil {
		return nil, err
	}
	return formatting.Decode(formatting.HexNC, reply.Bytes)
}

// ValidateAVMGenesis parses [genesis] exactly like the AVM does at chain
// initialisation, so that a malformed genesis is rejected before it is
// irrevocably registered on the M-chain.
func ValidateAVMGenesis(genesis []byte) error {
	if err := arc.ValidateGenesisEnvelope(constants.AVMID, genesis); err != nil {
		return err
	}
	parser, err := newAVMParser()
	if err != nil {
		return err
	}
	g := avm.Genesis{}
	if _, err := parser.GenesisCodec().Unmarshal(genesis, &g); err != nil {
		return fmt.Errorf("couldn't parse AVM genesis: %w", err)
	}
	if len(g.Txs) == 0 {
		return ErrGenesisNoAssets
	}
	for _, asset := range g.Txs {
		if len(asset.Outs) != 0 {
			return ErrGenesisAssetHasOuts
		}
		tx := avmtxs.Tx{Unsigned: &asset.CreateAssetTx}
		if err := parser.InitializeGenesisTx(&tx); err != nil {
			return fmt.Errorf("invalid genesis asset %q: %w", asset.Alias, err)
		}
	}
	return nil
}

// ValidateRequest performs every check that does not need the network.
func ValidateRequest(req Request) error {
	if err := arc.ValidateName(req.Name); err != nil {
		return err
	}
	if err := ValidateAVMGenesis(req.Genesis); err != nil {
		return err
	}
	if req.SubnetID == ids.Empty && req.Owner == nil {
		return ErrMissingOwner
	}
	if req.Owner != nil {
		if err := req.Owner.Verify(); err != nil {
			return fmt.Errorf("invalid owner: %w", err)
		}
	}
	if !req.ValidationEnd.After(req.ValidationStart) {
		return ErrInvalidPeriod
	}
	minSize := req.MinCommitteeSize
	if minSize == 0 {
		minSize = arc.DefaultMinCommitteeSize
	}
	if len(req.Committee) < minSize {
		return fmt.Errorf("%w: %d < %d", ErrCommitteeTooSmall, len(req.Committee), minSize)
	}
	seen := make(map[ids.NodeID]struct{}, len(req.Committee))
	for _, m := range req.Committee {
		if _, dup := seen[m.NodeID]; dup {
			return fmt.Errorf("%w: %s", ErrDuplicateMember, m.NodeID)
		}
		seen[m.NodeID] = struct{}{}
		if m.Weight == 0 {
			return fmt.Errorf("%w: %s", ErrZeroWeight, m.NodeID)
		}
	}
	return nil
}

// Create runs the workflow. [w] signs and issues transactions; [pClient] and
// [mcClient] read state from the same node.
func Create(
	ctx context.Context,
	w p.Wallet,
	pClient platformvm.Client,
	mcClient *arc.Client,
	req Request,
	options ...common.Option,
) (Result, error) {
	res := Result{SubnetID: req.SubnetID}
	if err := ValidateRequest(req); err != nil {
		return res, err
	}
	if err := checkEligibility(ctx, pClient, req); err != nil {
		return res, err
	}

	if res.SubnetID == ids.Empty {
		subnetID, err := w.IssueCreateSubnetTx(req.Owner, options...)
		if err != nil {
			return res, fmt.Errorf("couldn't create subnet: %w", err)
		}
		res.SubnetID = subnetID
		res.CreatedSubnet = true
	}

	existingChain, err := findExisting(ctx, pClient, mcClient, res.SubnetID, req)
	if err != nil {
		return res, err
	}

	members, err := subnetMembers(ctx, pClient, res.SubnetID)
	if err != nil {
		return res, err
	}
	for _, m := range req.Committee {
		if _, ok := members[m.NodeID]; ok {
			continue
		}
		txID, err := w.IssueAddSubnetValidatorTx(&validator.SubnetValidator{
			Validator: validator.Validator{
				NodeID: m.NodeID,
				Start:  uint64(req.ValidationStart.Unix()),
				End:    uint64(req.ValidationEnd.Unix()),
				Wght:   m.Weight,
			},
			Subnet: res.SubnetID,
		}, options...)
		if err != nil {
			return res, fmt.Errorf("couldn't add committee member %s: %w", m.NodeID, err)
		}
		res.ValidatorTxIDs = append(res.ValidatorTxIDs, txID)
	}

	if existingChain != ids.Empty {
		res.ChainID = existingChain
		return res, nil
	}
	// Register the chain only once its committee is active, so that nodes
	// never initialise it with an empty validator set.
	if err := awaitCommittee(ctx, pClient, res.SubnetID, req.Committee); err != nil {
		return res, err
	}
	chainID, err := w.IssueCreateChainTx(res.SubnetID, req.Genesis, constants.AVMID, FxIDs, req.Name, options...)
	if err != nil {
		return res, fmt.Errorf("couldn't create chain: %w", err)
	}
	res.ChainID = chainID
	res.CreatedChain = true
	return res, nil
}

func checkEligibility(ctx context.Context, pClient platformvm.Client, req Request) error {
	nodeIDs := make([]ids.NodeID, 0, len(req.Committee))
	for _, m := range req.Committee {
		nodeIDs = append(nodeIDs, m.NodeID)
	}
	primary, err := pClient.GetCurrentValidators(ctx, constants.PrimaryNetworkID, nodeIDs)
	if err != nil {
		return fmt.Errorf("couldn't fetch primary network validators: %w", err)
	}
	byID := make(map[ids.NodeID]platformvm.ClientPermissionlessValidator, len(primary))
	for _, v := range primary {
		byID[v.NodeID] = v
	}
	start, end := uint64(req.ValidationStart.Unix()), uint64(req.ValidationEnd.Unix())
	for _, m := range req.Committee {
		v, ok := byID[m.NodeID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotPrimaryValidator, m.NodeID)
		}
		if start < v.StartTime || end > v.EndTime {
			return fmt.Errorf("%w: %s", ErrPeriodNotBounded, m.NodeID)
		}
	}
	return nil
}

// findExisting returns the ID of a chain already registered on [subnetID]
// with the requested name, VM and genesis. It fails if the name is taken by a
// different chain on the subnet.
func findExisting(
	ctx context.Context,
	pClient platformvm.Client,
	mcClient *arc.Client,
	subnetID ids.ID,
	req Request,
) (ids.ID, error) {
	chains, err := pClient.GetBlockchains(ctx)
	if err != nil {
		return ids.Empty, fmt.Errorf("couldn't fetch blockchains: %w", err)
	}
	want := arc.FormatGenesisHash(arc.GenesisHash(req.Genesis))
	for _, c := range chains {
		if c.SubnetID != subnetID || c.Name != req.Name {
			continue
		}
		reply, err := mcClient.Get(ctx, c.ID)
		if err != nil {
			return ids.Empty, fmt.Errorf("couldn't fetch Arc %s: %w", c.ID, err)
		}
		if reply.Descriptor.GenesisHash != want || c.VMID != constants.AVMID {
			return ids.Empty, fmt.Errorf("%w: %s", ErrConflictingRegistered, c.ID)
		}
		return c.ID, nil
	}
	return ids.Empty, nil
}

const committeePollFrequency = 500 * time.Millisecond

func awaitCommittee(ctx context.Context, pClient platformvm.Client, subnetID ids.ID, committee []Member) error {
	ticker := time.NewTicker(committeePollFrequency)
	defer ticker.Stop()
	for {
		current, err := pClient.GetCurrentValidators(ctx, subnetID, nil)
		if err != nil {
			return fmt.Errorf("couldn't fetch subnet validators: %w", err)
		}
		active := make(map[ids.NodeID]struct{}, len(current))
		for _, v := range current {
			active[v.NodeID] = struct{}{}
		}
		missing := 0
		for _, m := range committee {
			if _, ok := active[m.NodeID]; !ok {
				missing++
			}
		}
		if missing == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("committee not active (%d members pending): %w", missing, ctx.Err())
		case <-ticker.C:
		}
	}
}

func subnetMembers(ctx context.Context, pClient platformvm.Client, subnetID ids.ID) (map[ids.NodeID]struct{}, error) {
	members := make(map[ids.NodeID]struct{})
	current, err := pClient.GetCurrentValidators(ctx, subnetID, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch subnet validators: %w", err)
	}
	for _, v := range current {
		members[v.NodeID] = struct{}{}
	}
	pending, _, err := pClient.GetPendingValidators(ctx, subnetID, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch pending subnet validators: %w", err)
	}
	for _, raw := range pending {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		nodeIDStr, ok := entry["nodeID"].(string)
		if !ok {
			continue
		}
		nodeID, err := ids.NodeIDFromString(nodeIDStr)
		if err != nil {
			continue
		}
		members[nodeID] = struct{}{}
	}
	return members, nil
}
