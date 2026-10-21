package arc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/nftfx"
	"github.com/lasthyphen/dijetsnodesgo/vms/propertyfx"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

const (
	// APIVersion is the version of the arc.* JSON-RPC API.
	APIVersion = "v1"
	// SchemaVersion is the version of the descriptor/response schemas.
	// Bump on any backwards-incompatible change.
	SchemaVersion uint32 = 1
	// ProtocolVersion identifies the Arc protocol: a CreateChainTx on
	// a dedicated permissioned subnet, registered on the M-chain.
	ProtocolVersion = "arc/1"
	// MetadataVersion is the version of the metadata derivation rules.
	MetadataVersion uint32 = 1

	// MaxNameLen is the maximum name length accepted by the creation
	// workflow. The protocol allows up to 128 characters.
	MaxNameLen = 64

	// RegistryAuthority is the authoritative source of registration data.
	RegistryAuthority = "M-chain"
)

// PrivacyMode is a declared privacy configuration. It is not a claim of
// cryptographic confidentiality.
type PrivacyMode string

// PrivacyPublic: chain data is replicated to every committee member and can
// be served by any node running the chain. Nothing is encrypted.
const PrivacyPublic PrivacyMode = "public"

var (
	ErrNotFound        = errors.New("Arc not found")
	ErrNotAnArc        = errors.New("chain is not an Arc")
	errEmptyName       = errors.New("name must not be empty")
	errNameTooLong     = fmt.Errorf("name must be at most %d characters", MaxNameLen)
	errNameFirstChar   = errors.New("name must start with an ASCII letter")
	errNameIllegalChar = errors.New("name may contain only ASCII letters, digits and single spaces")
	errNameEdgeSpace   = errors.New("name must not end with a space")
	errNameDoubleSpace = errors.New("name must not contain consecutive spaces")
	errEmptyGenesis    = errors.New("genesis must not be empty")
	errGenesisTooLarge = errors.New("genesis exceeds 1 MiB")
	errUnsupportedVM   = errors.New("vm is not supported for Arcs")
	maxGenesisBytes    = 1 << 20
	supportedArcVM     = constants.AVMID
)

// ValidateName applies the Arc naming policy. It is stricter than the
// protocol rule (ASCII letters, digits and spaces, at most 128 characters).
// Names are not unique: the protocol does not enforce uniqueness.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errEmptyName
	case len(name) > MaxNameLen:
		return errNameTooLong
	case strings.HasSuffix(name, " "):
		return errNameEdgeSpace
	case strings.Contains(name, "  "):
		return errNameDoubleSpace
	}
	first := rune(name[0])
	if first > unicode.MaxASCII || !unicode.IsLetter(first) {
		return errNameFirstChar
	}
	for _, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ') {
			return errNameIllegalChar
		}
	}
	return nil
}

// ValidateGenesisEnvelope checks the VM and the size of the genesis. VM
// specific parsing is done by the VM-aware workflow before submission.
func ValidateGenesisEnvelope(vmID ids.ID, genesis []byte) error {
	switch {
	case vmID != supportedArcVM:
		return fmt.Errorf("%w: %s", errUnsupportedVM, vmID)
	case len(genesis) == 0:
		return errEmptyGenesis
	case len(genesis) > maxGenesisBytes:
		return errGenesisTooLarge
	}
	return nil
}

// GenesisHash returns the canonical genesis hash of an Arc.
func GenesisHash(genesis []byte) [32]byte {
	return sha256.Sum256(genesis)
}

// FormatGenesisHash formats a genesis hash for API responses.
func FormatGenesisHash(h [32]byte) string {
	return "sha256:" + hex.EncodeToString(h[:])
}

// SupportedVMs lists the VMs accepted for new Arcs.
func SupportedVMs() []ids.ID {
	return []ids.ID{supportedArcVM}
}

// Owner is the public, M-chain recorded owner of an Arc's subnet. Only
// addresses are exposed; no key material exists in M-chain state.
type Owner struct {
	Threshold uint32   `json:"threshold"`
	Locktime  uint64   `json:"locktime"`
	Addresses []string `json:"addresses"`
}

// Contains reports whether [address] is one of the owner's addresses.
func (o Owner) Contains(address string) bool {
	for _, a := range o.Addresses {
		if a == address {
			return true
		}
	}
	return false
}

// Record is an Arc's authoritative registration as read from the
// M-chain state.
type Record struct {
	ChainID          ids.ID
	SubnetID         ids.ID
	Name             string
	VMID             ids.ID
	FxIDs            []ids.ID
	GenesisHash      [32]byte
	GenesisSize      int
	Owner            Owner
	Transformed      bool
	SubnetChainCount int
}

// Profile returns the security profile of the record's subnet.
func (r Record) Profile() Profile {
	if r.Transformed {
		return PermissionlessSubnet
	}
	return PermissionedCommittee
}

// TxStatus is the M-chain status of a transaction.
type TxStatus string

const (
	TxCommitted  TxStatus = "Committed"
	TxProcessing TxStatus = "Processing"
	TxAborted    TxStatus = "Aborted"
	TxDropped    TxStatus = "Dropped"
	TxUnknown    TxStatus = "Unknown"
)

// Registry is a read-only view over the authoritative M-chain state. Every
// method reads committed state only.
type Registry interface {
	NetworkID() uint32
	StakingEnabled() bool
	// Bootstrapped reports whether the M-chain finished bootstrapping. Before
	// that, registration data may be incomplete.
	Bootstrapped() bool
	// Chains returns every Arc sorted by chain ID, and the M-chain
	// height the data was read at.
	Chains(ctx context.Context) ([]Record, uint64, error)
	// Chain returns ErrNotFound if [chainID] is unknown and ErrNotAnArc
	// if it is a primary network chain.
	Chain(ctx context.Context, chainID ids.ID) (Record, uint64, error)
	// Committee returns current and pending members of [subnetID].
	Committee(ctx context.Context, subnetID ids.ID) ([]Member, error)
	// TracksSubnet reports whether this node is configured to run chains of
	// [subnetID].
	TracksSubnet(subnetID ids.ID) bool
	TxStatus(ctx context.Context, txID ids.ID) (TxStatus, error)
	// LookupChain resolves a chain ID or alias.
	LookupChain(alias string) (ids.ID, error)
}

// Registration describes the authoritative registration of an Arc.
type Registration struct {
	Status                 string `json:"status"`
	Authority              string `json:"authority"`
	ObservedAtMChainHeight uint64 `json:"observedAtMChainHeight"`
}

// ValidatorSummary summarises configured committee membership.
type ValidatorSummary struct {
	CurrentCount  int    `json:"currentCount"`
	PendingCount  int    `json:"pendingCount"`
	CurrentWeight uint64 `json:"currentWeight"`
	Source        string `json:"source"`
}

// PolicyView is the security profile of an Arc.
type PolicyView struct {
	Profile     Profile      `json:"profile"`
	Enforcement Enforcement  `json:"enforcement"`
	Params      PolicyParams `json:"params"`
	Provides    []string     `json:"provides"`
}

// CreatedBy identifies the transactions that created an Arc.
type CreatedBy struct {
	// ChainTxID is the CreateChainTx ID. It equals the chain ID.
	ChainTxID ids.ID `json:"chainTxID"`
	// SubnetTxID is the CreateSubnetTx ID. It equals the subnet ID.
	SubnetTxID ids.ID `json:"subnetTxID"`
}

// Descriptor is the versioned description of an Arc. Every field is
// derived from authoritative M-chain state, except RPCEndpoints which
// reflects what this node serves.
type Descriptor struct {
	SchemaVersion   uint32 `json:"schemaVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	MetadataVersion uint32 `json:"metadataVersion"`

	ID          ids.ID   `json:"id"`
	Name        string   `json:"name"`
	SubnetID    ids.ID   `json:"subnetID"`
	NetworkID   uint32   `json:"networkID"`
	VMID        ids.ID   `json:"vmID"`
	VMName      string   `json:"vmName"`
	FxIDs       []ids.ID `json:"fxIDs"`
	GenesisHash string   `json:"genesisHash"`
	GenesisSize int      `json:"genesisSize"`

	Owner     Owner      `json:"owner"`
	CreatedBy CreatedBy  `json:"createdBy"`
	CreatedAt *time.Time `json:"createdAt"`

	ValidatorPolicy PolicyView       `json:"validatorPolicy"`
	Validators      ValidatorSummary `json:"validators"`
	// SubnetChainCount is the number of chains sharing this chain's subnet,
	// and therefore its committee. 1 for a dedicated subnet.
	SubnetChainCount int `json:"subnetChainCount"`

	RPCEndpoints []string     `json:"rpcEndpoints"`
	ExplorerURL  *string      `json:"explorerURL"`
	PrivacyMode  PrivacyMode  `json:"privacyMode"`
	Capabilities []string     `json:"capabilities"`
	Registration Registration `json:"registration"`
	// Unavailable lists descriptor fields that have no data, with the reason.
	Unavailable map[string]string `json:"unavailable"`
}

// Unavailability reasons.
const (
	UnavailableNotIndexed       = "not_recorded_in_authoritative_state"
	UnavailableNotConfigured    = "not_configured"
	UnavailableNotServedLocally = "chain_not_served_by_this_node"
	UnavailableNoLinearHeight   = "dag_chain_has_no_linear_height"
	UnavailableNotObserved      = "not_observed_since_node_start"
	UnavailableNotTracked       = "node_not_tracking_subnet"
)

// VMName returns a human readable VM name.
func VMName(vmID ids.ID) string {
	switch vmID {
	case constants.AVMID:
		return "avm"
	case constants.EVMID:
		return "evm"
	case constants.PlatformVMID:
		return "platformvm"
	default:
		return "unknown"
	}
}

var fxNames = map[ids.ID]string{
	secp256k1fx.ID: "secp256k1fx",
	nftfx.ID:       "nftfx",
	propertyfx.ID:  "propertyfx",
}

// Capabilities derives the execution capabilities of a chain from its VM and
// feature extensions.
func Capabilities(vmID ids.ID, fxIDs []ids.ID) []string {
	var caps []string
	switch vmID {
	case constants.AVMID:
		caps = append(caps,
			"vm:avm",
			"consensus:dag",
			"state:utxo",
			"assets:fungible",
			"tx:base",
			"tx:create_asset",
			"tx:operation",
			"api:avm",
			"api:wallet",
			"api:events",
			"atomic:disabled_across_subnets",
		)
	default:
		caps = append(caps, "vm:"+VMName(vmID))
	}
	for _, fxID := range fxIDs {
		name, ok := fxNames[fxID]
		if !ok {
			name = fxID.String()
		}
		caps = append(caps, "fx:"+name)
	}
	return caps
}

// BuildDescriptor builds a descriptor from authoritative data.
func BuildDescriptor(
	rec Record,
	height uint64,
	networkID uint32,
	eval CommitteeEvaluation,
	params PolicyParams,
	servedLocally bool,
) Descriptor {
	info, _ := ProfileInfoFor(eval.Profile)
	fxIDs := rec.FxIDs
	if fxIDs == nil {
		fxIDs = []ids.ID{}
	}
	if rec.Owner.Addresses == nil {
		rec.Owner.Addresses = []string{}
	}
	d := Descriptor{
		SchemaVersion:   SchemaVersion,
		ProtocolVersion: ProtocolVersion,
		MetadataVersion: MetadataVersion,
		ID:              rec.ChainID,
		Name:            rec.Name,
		SubnetID:        rec.SubnetID,
		NetworkID:       networkID,
		VMID:            rec.VMID,
		VMName:          VMName(rec.VMID),
		FxIDs:           fxIDs,
		GenesisHash:     FormatGenesisHash(rec.GenesisHash),
		GenesisSize:     rec.GenesisSize,
		Owner:           rec.Owner,
		CreatedBy: CreatedBy{
			ChainTxID:  rec.ChainID,
			SubnetTxID: rec.SubnetID,
		},
		ValidatorPolicy: PolicyView{
			Profile:     eval.Profile,
			Enforcement: eval.Enforcement,
			Params:      params,
			Provides:    info.Provides,
		},
		Validators: ValidatorSummary{
			CurrentCount:  eval.CurrentSize,
			PendingCount:  eval.PendingSize,
			CurrentWeight: eval.CurrentWeight,
			Source:        RegistryAuthority,
		},
		SubnetChainCount: rec.SubnetChainCount,
		RPCEndpoints:     []string{},
		PrivacyMode:      PrivacyPublic,
		Capabilities:     Capabilities(rec.VMID, rec.FxIDs),
		Registration: Registration{
			Status:                 "REGISTERED",
			Authority:              RegistryAuthority,
			ObservedAtMChainHeight: height,
		},
		Unavailable: map[string]string{
			"createdAt":   UnavailableNotIndexed,
			"explorerURL": UnavailableNotConfigured,
		},
	}
	if d.ValidatorPolicy.Provides == nil {
		d.ValidatorPolicy.Provides = []string{}
	}
	if servedLocally {
		d.RPCEndpoints = []string{fmt.Sprintf("/ext/%s/%s", constants.ChainAliasPrefix, rec.ChainID)}
	} else {
		d.Unavailable["rpcEndpoints"] = UnavailableNotServedLocally
	}
	return d
}
