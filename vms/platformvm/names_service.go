package platformvm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"

	"go.uber.org/zap"

	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

const (
	// NamesServiceName is the JSON-RPC service name: methods are
	// "names.<method>".
	NamesServiceName = "names"
	// NamesEndpoint is the extension under which the service is mounted on
	// the M-chain: /ext/bc/M/names.
	NamesEndpoint = "/names"

	NamesAPIVersion    = "v1"
	NamesSchemaVersion = 1

	maxConcurrentNameRequests = 32
	nameRequestAcquireTimeout = time.Second
)

// JSON-RPC error codes returned by the names service. They are stable
// across API versions.
const (
	NamesCodeInternal      json2.ErrorCode = -32000
	NamesCodeInvalidParams json2.ErrorCode = -32602
	NamesCodeBusy          json2.ErrorCode = -32029
)

// NamesErrorData is attached to every error returned by the names service.
type NamesErrorData struct {
	APIVersion string `json:"apiVersion"`
	Reason     string `json:"reason"`
}

func namesError(code json2.ErrorCode, reason, msg string) *json2.Error {
	return &json2.Error{
		Code:    code,
		Message: msg,
		Data:    NamesErrorData{APIVersion: NamesAPIVersion, Reason: reason},
	}
}

// NamesEnvelope versions every reply.
type NamesEnvelope struct {
	APIVersion    string `json:"apiVersion"`
	SchemaVersion int    `json:"schemaVersion"`
}

// StatePoint identifies the accepted M-chain state an answer was computed
// from. All honest nodes give the same answer for the same block.
type StatePoint struct {
	BlockID   ids.ID      `json:"blockID"`
	Height    json.Uint64 `json:"height"`
	ChainTime json.Uint64 `json:"chainTime"`
}

// NameOwner is the k-of-n set that may update a registration.
type NameOwner struct {
	Threshold json.Uint32 `json:"threshold"`
	Locktime  json.Uint64 `json:"locktime"`
	Addresses []string    `json:"addresses"`
}

// NameRecord is one version of a registration.
type NameRecord struct {
	Name     string      `json:"name"`
	Status   string      `json:"status"`
	Target   string      `json:"target"`
	TargetID ids.ShortID `json:"targetID"`
	Owner    NameOwner   `json:"owner"`
	// Expiry, RegisteredAt and UpdatedAt are Unix seconds of M-chain time.
	Expiry       json.Uint64 `json:"expiry"`
	Revoked      bool        `json:"revoked"`
	Generation   json.Uint64 `json:"generation"`
	Version      json.Uint64 `json:"version"`
	RecordID     ids.ID      `json:"recordID"`
	LastTxID     ids.ID      `json:"lastTxID"`
	RegisteredAt json.Uint64 `json:"registeredAt"`
	UpdatedAt    json.Uint64 `json:"updatedAt"`
}

type ResolveNameArgs struct {
	Name string `json:"name"`
}

// NameResolution is the answer to names.resolve. Record is nil when the
// name has never been registered.
type NameResolution struct {
	NamesEnvelope
	Input      string      `json:"input"`
	Name       string      `json:"name"`
	Status     string      `json:"status"`
	Resolvable bool        `json:"resolvable"`
	Record     *NameRecord `json:"record,omitempty"`
	At         StatePoint  `json:"at"`
}

type ReverseNameArgs struct {
	Address string `json:"address"`
}

// ReverseResolution is the answer to names.reverse. Name is empty unless an
// ACTIVE registration targets the address.
type ReverseResolution struct {
	NamesEnvelope
	Address string      `json:"address"`
	Name    string      `json:"name,omitempty"`
	Record  *NameRecord `json:"record,omitempty"`
	At      StatePoint  `json:"at"`
}

type NameHistoryReply struct {
	NamesEnvelope
	Name     string        `json:"name"`
	Versions []*NameRecord `json:"versions"`
	At       StatePoint    `json:"at"`
}

type ValidateNameReply struct {
	NamesEnvelope
	Input     string `json:"input"`
	Canonical string `json:"canonical"`
}

type NameParamsReply struct {
	NamesEnvelope
	Active         bool        `json:"active"`
	ActivationTime json.Uint64 `json:"activationTime"`
	MinTerm        json.Uint64 `json:"minTermSeconds"`
	MaxTerm        json.Uint64 `json:"maxTermSeconds"`
	GracePeriod    json.Uint64 `json:"gracePeriodSeconds"`
	RegisterFee    json.Uint64 `json:"registerFee"`
	UpdateFee      json.Uint64 `json:"updateFee"`
	MinLength      int         `json:"minLength"`
	MaxLength      int         `json:"maxLength"`
	At             StatePoint  `json:"at"`
}

// NamesService is the read-only names.* JSON-RPC service. Every answer is
// computed from the last accepted M-chain state, never from processing
// blocks.
type NamesService struct {
	vm      *VM
	limiter chan struct{}
}

func newNamesHandler(vm *VM) (http.Handler, error) {
	server := rpc.NewServer()
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")
	server.RegisterInterceptFunc(vm.metrics.InterceptRequest)
	server.RegisterAfterFunc(vm.metrics.AfterRequest)
	service := &NamesService{
		vm:      vm,
		limiter: make(chan struct{}, maxConcurrentNameRequests),
	}
	return server, server.RegisterService(service, NamesServiceName)
}

var envelope = NamesEnvelope{APIVersion: NamesAPIVersion, SchemaVersion: NamesSchemaVersion}

// begin bounds concurrency and takes the VM lock, so that each answer
// reflects one consistent accepted state.
func (s *NamesService) begin() (func(), error) {
	timer := time.NewTimer(nameRequestAcquireTimeout)
	defer timer.Stop()
	select {
	case s.limiter <- struct{}{}:
	case <-timer.C:
		return nil, namesError(NamesCodeBusy, "too_many_concurrent_requests", "service busy, retry later")
	}
	s.vm.ctx.Lock.Lock()
	return func() {
		s.vm.ctx.Lock.Unlock()
		<-s.limiter
	}, nil
}

func (s *NamesService) internal(method string, err error) error {
	s.vm.ctx.Log.Warn("names API request failed",
		zap.String("method", method),
		zap.Error(err),
	)
	return namesError(NamesCodeInternal, "internal_error", "internal error")
}

func invalidName(err error) error {
	var nameErr *names.Error
	if errors.As(err, &nameErr) {
		return namesError(NamesCodeInvalidParams, "invalid_name:"+string(nameErr.Rule), err.Error())
	}
	return namesError(NamesCodeInvalidParams, "invalid_name", err.Error())
}

func (s *NamesService) statePoint() (StatePoint, error) {
	blkID := s.vm.state.GetLastAccepted()
	blk, _, err := s.vm.state.GetStatelessBlock(blkID)
	if err != nil {
		return StatePoint{}, err
	}
	return StatePoint{
		BlockID:   blkID,
		Height:    json.Uint64(blk.Height()),
		ChainTime: json.Uint64(s.vm.state.GetTimestamp().Unix()),
	}, nil
}

func (s *NamesService) formatAddress(chainID ids.ID, addr ids.ShortID) (string, error) {
	alias, err := s.vm.ctx.BCLookup.PrimaryAlias(chainID)
	if err != nil {
		return "", err
	}
	hrp := constants.GetHRP(s.vm.ctx.NetworkID)
	return address.Format(alias, hrp, addr[:])
}

func (s *NamesService) recordJSON(record *names.Record) (*NameRecord, error) {
	target, err := s.formatAddress(s.vm.ctx.XChainID, record.Target)
	if err != nil {
		return nil, err
	}
	owner, ok := record.Owner.(*secp256k1fx.OutputOwners)
	if !ok {
		return nil, fmt.Errorf("unexpected owner type %T", record.Owner)
	}
	ownerAddrs := make([]string, len(owner.Addrs))
	for i, addr := range owner.Addrs {
		ownerAddrs[i], err = s.formatAddress(s.vm.ctx.ChainID, addr)
		if err != nil {
			return nil, err
		}
	}
	status := record.StatusAt(s.vm.state.GetTimestamp(), s.vm.NamingParams.GracePeriod)
	return &NameRecord{
		Name:     record.Name,
		Status:   string(status),
		Target:   target,
		TargetID: record.Target,
		Owner: NameOwner{
			Threshold: json.Uint32(owner.Threshold),
			Locktime:  json.Uint64(owner.Locktime),
			Addresses: ownerAddrs,
		},
		Expiry:       json.Uint64(record.Expiry),
		Revoked:      record.Revoked,
		Generation:   json.Uint64(record.Generation),
		Version:      json.Uint64(record.Version),
		RecordID:     record.RecordID,
		LastTxID:     record.LastTxID,
		RegisteredAt: json.Uint64(record.RegisteredAt),
		UpdatedAt:    json.Uint64(record.UpdatedAt),
	}, nil
}

// Resolve returns the registration of a name and whether payments may be
// directed to its target.
func (s *NamesService) Resolve(_ *http.Request, args *ResolveNameArgs, reply *NameResolution) error {
	name, err := names.Canonicalise(args.Name)
	if err != nil {
		return invalidName(err)
	}

	done, err := s.begin()
	if err != nil {
		return err
	}
	defer done()

	at, err := s.statePoint()
	if err != nil {
		return s.internal("resolve", err)
	}
	*reply = NameResolution{
		NamesEnvelope: envelope,
		Input:         args.Name,
		Name:          name,
		Status:        string(names.StatusNotFound),
		At:            at,
	}

	record, err := s.vm.state.GetName(name)
	if err == database.ErrNotFound {
		return nil
	}
	if err != nil {
		return s.internal("resolve", err)
	}
	reply.Record, err = s.recordJSON(record)
	if err != nil {
		return s.internal("resolve", err)
	}
	reply.Status = reply.Record.Status
	reply.Resolvable = names.Status(reply.Status).Resolvable()
	return nil
}

// Reverse returns the name whose ACTIVE registration targets an address.
// The address may carry any chain prefix ("V-", "M-") or none, because a
// key has the same identity on every chain.
func (s *NamesService) Reverse(_ *http.Request, args *ReverseNameArgs, reply *ReverseResolution) error {
	target, err := s.parseIdentity(args.Address)
	if err != nil {
		return namesError(NamesCodeInvalidParams, "invalid_address", err.Error())
	}

	done, err := s.begin()
	if err != nil {
		return err
	}
	defer done()

	at, err := s.statePoint()
	if err != nil {
		return s.internal("reverse", err)
	}
	*reply = ReverseResolution{
		NamesEnvelope: envelope,
		Address:       args.Address,
		At:            at,
	}

	name, err := s.vm.state.GetNameTarget(target)
	if err == database.ErrNotFound {
		return nil
	}
	if err != nil {
		return s.internal("reverse", err)
	}
	record, err := s.vm.state.GetName(name)
	if err != nil {
		return s.internal("reverse", err)
	}
	// The reverse entry can be stale; only a live binding counts.
	status := record.StatusAt(s.vm.state.GetTimestamp(), s.vm.NamingParams.GracePeriod)
	if record.Target != target || !status.Resolvable() {
		return nil
	}
	reply.Name = name
	reply.Record, err = s.recordJSON(record)
	if err != nil {
		return s.internal("reverse", err)
	}
	return nil
}

func (s *NamesService) parseIdentity(addr string) (ids.ShortID, error) {
	bech32 := addr
	if i := strings.IndexByte(addr, '-'); i >= 0 {
		bech32 = addr[i+1:]
	}
	hrp, payload, err := address.ParseBech32(bech32)
	if err != nil {
		return ids.ShortID{}, err
	}
	if expected := constants.GetHRP(s.vm.ctx.NetworkID); hrp != expected {
		return ids.ShortID{}, fmt.Errorf("expected hrp %q but got %q", expected, hrp)
	}
	return ids.ToShortID(payload)
}

// History returns every committed version of a name, oldest first,
// including earlier generations.
func (s *NamesService) History(_ *http.Request, args *ResolveNameArgs, reply *NameHistoryReply) error {
	name, err := names.Canonicalise(args.Name)
	if err != nil {
		return invalidName(err)
	}

	done, err := s.begin()
	if err != nil {
		return err
	}
	defer done()

	at, err := s.statePoint()
	if err != nil {
		return s.internal("history", err)
	}
	history, err := s.vm.state.GetNameHistory(name)
	if err != nil {
		return s.internal("history", err)
	}
	*reply = NameHistoryReply{
		NamesEnvelope: envelope,
		Name:          name,
		Versions:      make([]*NameRecord, len(history)),
		At:            at,
	}
	for i, record := range history {
		reply.Versions[i], err = s.recordJSON(record)
		if err != nil {
			return s.internal("history", err)
		}
		// Only the latest version carries the record's current status.
		if i < len(history)-1 {
			reply.Versions[i].Status = "SUPERSEDED"
		}
	}
	return nil
}

// Validate returns the canonical form of a name, or the rule it breaks.
// It does not say whether the name is available.
func (*NamesService) Validate(_ *http.Request, args *ResolveNameArgs, reply *ValidateNameReply) error {
	name, err := names.Canonicalise(args.Name)
	if err != nil {
		return invalidName(err)
	}
	*reply = ValidateNameReply{
		NamesEnvelope: envelope,
		Input:         args.Name,
		Canonical:     name,
	}
	return nil
}

// Params returns the registry's parameters and whether it is active.
func (s *NamesService) Params(_ *http.Request, _ *struct{}, reply *NameParamsReply) error {
	done, err := s.begin()
	if err != nil {
		return err
	}
	defer done()

	at, err := s.statePoint()
	if err != nil {
		return s.internal("params", err)
	}
	params := s.vm.NamingParams
	activation := s.vm.NamingTime
	if s.vm.BanffTime.After(activation) {
		activation = s.vm.BanffTime
	}
	*reply = NameParamsReply{
		NamesEnvelope:  envelope,
		Active:         s.vm.IsNamingActivated(s.vm.state.GetTimestamp()),
		ActivationTime: json.Uint64(activation.Unix()),
		MinTerm:        json.Uint64(params.MinTerm / time.Second),
		MaxTerm:        json.Uint64(params.MaxTerm / time.Second),
		GracePeriod:    json.Uint64(params.GracePeriod / time.Second),
		RegisterFee:    json.Uint64(params.RegisterFee),
		UpdateFee:      json.Uint64(s.vm.TxFee),
		MinLength:      names.MinLength,
		MaxLength:      names.MaxLength,
		At:             at,
	}
	return nil
}
