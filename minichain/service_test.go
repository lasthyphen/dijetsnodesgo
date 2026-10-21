package minichain

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/chains"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"
)

type fakeRegistry struct {
	staking    bool
	records    []Record
	committees map[ids.ID][]Member
	tracked    map[ids.ID]bool
	txs        map[ids.ID]TxStatus
	failChains bool
}

func (*fakeRegistry) NetworkID() uint32      { return 1337 }
func (r *fakeRegistry) StakingEnabled() bool { return r.staking }
func (*fakeRegistry) Bootstrapped() bool     { return true }

func (r *fakeRegistry) Chains(context.Context) ([]Record, uint64, error) {
	if r.failChains {
		return nil, 0, errors.New("disk on fire: /secret/path")
	}
	out := append([]Record(nil), r.records...)
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i].ChainID[:], out[j].ChainID[:]) < 0 })
	return out, 42, nil
}

func (r *fakeRegistry) Chain(_ context.Context, chainID ids.ID) (Record, uint64, error) {
	if chainID == constants.PlatformChainID {
		return Record{}, 0, ErrNotAMiniChain
	}
	for _, rec := range r.records {
		if rec.ChainID == chainID {
			return rec, 42, nil
		}
	}
	return Record{}, 0, ErrNotFound
}

func (r *fakeRegistry) Committee(_ context.Context, subnetID ids.ID) ([]Member, error) {
	return r.committees[subnetID], nil
}

func (r *fakeRegistry) TracksSubnet(subnetID ids.ID) bool { return r.tracked[subnetID] }

func (r *fakeRegistry) TxStatus(_ context.Context, txID ids.ID) (TxStatus, error) {
	if s, ok := r.txs[txID]; ok {
		return s, nil
	}
	return TxUnknown, nil
}

func (*fakeRegistry) LookupChain(alias string) (ids.ID, error) {
	if alias == "M" {
		return constants.PlatformChainID, nil
	}
	return ids.FromString(alias)
}

type harness struct {
	registry *fakeRegistry
	observer *Observer
	service  *Service
	server   *httptest.Server
	local    map[ids.ID]chains.LocalChainStatus
	active   Record
	failed   Record
	remote   Record
}

func connectedMembers(n int, connected bool) []Member {
	members := make([]Member, n)
	for i := range members {
		c := connected
		members[i] = Member{
			NodeID:    ids.GenerateTestNodeID(),
			TxID:      ids.GenerateTestID(),
			Weight:    20,
			StartTime: time.Unix(1000, 0),
			EndTime:   time.Unix(2000, 0),
			Status:    MemberCurrent,
			Connected: &c,
		}
	}
	return members
}

func newHarness(t *testing.T) *harness {
	require := require.New(t)

	active, failed, remote := testRecord(), testRecord(), testRecord()
	active.Name, failed.Name, remote.Name = "active", "failed", "remote"
	remote.Owner = Owner{Threshold: 1, Addresses: []string{"M-local1remoteowner"}}

	registry := &fakeRegistry{
		staking: true,
		records: []Record{active, failed, remote},
		committees: map[ids.ID][]Member{
			active.SubnetID: connectedMembers(3, true),
			failed.SubnetID: connectedMembers(3, true),
			remote.SubnetID: {{NodeID: ids.GenerateTestNodeID(), Weight: 1, Status: MemberCurrent}},
		},
		tracked: map[ids.ID]bool{active.SubnetID: true, failed.SubnetID: true},
		txs:     map[ids.ID]TxStatus{},
	}

	decisions := snow.NewAcceptorGroup(logging.NoLog{})
	consensus := snow.NewAcceptorGroup(logging.NoLog{})
	observer, err := NewObserver(ObserverConfig{
		Log:                    logging.NoLog{},
		NodeID:                 ids.GenerateTestNodeID(),
		DecisionAcceptorGroup:  decisions,
		ConsensusAcceptorGroup: consensus,
	})
	require.NoError(err)

	h := &harness{
		registry: registry,
		observer: observer,
		local: map[ids.ID]chains.LocalChainStatus{
			active.ChainID: {ChainID: active.ChainID, Created: true, State: snow.NormalOp},
			failed.ChainID: {ChainID: failed.ChainID, CreationErr: errors.New("couldn't open /var/db")},
		},
		active: active,
		failed: failed,
		remote: remote,
	}
	observer.SetLocalStatusSource(func(chainID ids.ID) (chains.LocalChainStatus, bool) {
		s, ok := h.local[chainID]
		return s, ok
	})
	// The active chain is registered through the chain manager's registrant
	// path, with a healthy engine, and accepts 3 txs in 2 vertices.
	chainCtx := snow.DefaultConsensusContextTest()
	chainCtx.ChainID = active.ChainID
	chainCtx.SubnetID = active.SubnetID
	chainCtx.SetState(snow.NormalOp)
	engine := &common.EngineTest{
		T:        t,
		ContextF: func() *snow.ConsensusContext { return chainCtx },
		HealthF:  func(context.Context) (interface{}, error) { return nil, nil },
	}
	observer.RegisterChain("active", engine)
	// Primary network chains are ignored.
	primaryCtx := snow.DefaultConsensusContextTest()
	primaryCtx.ChainID = ids.GenerateTestID()
	observer.RegisterChain("V", &common.EngineTest{T: t, ContextF: func() *snow.ConsensusContext { return primaryCtx }})
	_, tracked := observer.Stats(primaryCtx.ChainID)
	require.False(tracked)

	for i := 0; i < 3; i++ {
		require.NoError(decisions.Accept(chainCtx, ids.GenerateTestID(), nil))
	}
	for i := 0; i < 2; i++ {
		require.NoError(consensus.Accept(chainCtx, ids.GenerateTestID(), nil))
	}

	h.service = NewService(logging.NoLog{}, registry, observer)
	observer.Poll(context.Background())
	handler, err := NewHandler(h.service)
	require.NoError(err)
	h.server = httptest.NewServer(handler)
	t.Cleanup(h.server.Close)
	return h
}

type rpcResponse struct {
	Result stdjson.RawMessage `json:"result"`
	Error  *struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    ErrorData `json:"data"`
	} `json:"error"`
}

func (h *harness) call(t *testing.T, method string, params interface{}, result interface{}) *rpcResponse {
	body, err := stdjson.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)
	resp, err := http.Post(h.server.URL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var out rpcResponse
	require.NoError(t, stdjson.NewDecoder(resp.Body).Decode(&out))
	if out.Error == nil && result != nil {
		require.NoError(t, stdjson.Unmarshal(out.Result, result))
	}
	return &out
}

func TestServiceList(t *testing.T) {
	require := require.New(t)
	h := newHarness(t)

	var reply ListReply
	resp := h.call(t, "minichain.list", map[string]interface{}{}, &reply)
	require.Nil(resp.Error)
	require.Equal(APIVersion, reply.APIVersion)
	require.Equal(SchemaVersion, reply.SchemaVersion)
	require.Len(reply.Chains, 3)
	require.Empty(reply.NextCursor)
	require.Equal(uint64(42), reply.MChainHeight)

	counts := reply.Counts
	require.Equal(3, counts.Registered)
	require.Equal(2, counts.Tracked)
	require.Equal(1, counts.Operational)
	require.Equal(1, counts.Healthy)
	require.Equal(1, counts.ByLifecycle[Active])
	require.Equal(1, counts.ByLifecycle[Failed])
	require.Equal(1, counts.ByLifecycle[Unknown])
	require.Equal(0, counts.ByLifecycle[Pending])

	byName := map[string]ChainView{}
	for _, c := range reply.Chains {
		byName[c.Descriptor.Name] = c
		require.True(c.Observation.Registered)
		require.Equal(SourceLocalObservation, c.Observation.Source)
	}
	require.Equal(Active, byName["active"].Observation.Lifecycle)
	require.True(byName["active"].Observation.Healthy)
	require.NotEmpty(byName["active"].Descriptor.RPCEndpoints)

	require.Equal(Failed, byName["failed"].Observation.Lifecycle)
	require.Equal([]string{ReasonInitialisationFailed}, byName["failed"].Observation.Reasons)
	require.Empty(byName["failed"].Descriptor.RPCEndpoints)

	// A registered chain this node does not run is never reported healthy.
	remote := byName["remote"].Observation
	require.Equal(Unknown, remote.Lifecycle)
	require.False(remote.Operational)
	require.False(remote.Healthy)
	require.False(remote.TrackedByNode)
}

func TestServiceListPaginationAndFilters(t *testing.T) {
	require := require.New(t)
	h := newHarness(t)

	var seen []ids.ID
	cursor := ""
	for i := 0; i < 5; i++ {
		var reply ListReply
		resp := h.call(t, "minichain.list", map[string]interface{}{"limit": 1, "cursor": cursor}, &reply)
		require.Nil(resp.Error)
		for _, c := range reply.Chains {
			seen = append(seen, c.Descriptor.ID)
		}
		require.Equal(3, reply.Counts.Matched)
		if reply.NextCursor == "" {
			break
		}
		cursor = reply.NextCursor
	}
	require.Len(seen, 3)
	require.True(sort.SliceIsSorted(seen, func(i, j int) bool { return bytes.Compare(seen[i][:], seen[j][:]) < 0 }))

	var filtered ListReply
	require.Nil(h.call(t, "minichain.list", map[string]interface{}{"lifecycle": "FAILED"}, &filtered).Error)
	require.Len(filtered.Chains, 1)
	require.Equal("failed", filtered.Chains[0].Descriptor.Name)
	require.Equal(3, filtered.Counts.Registered)

	var byOwner ListReply
	require.Nil(h.call(t, "minichain.list", map[string]interface{}{"owner": "local1remoteowner"}, &byOwner).Error)
	require.Len(byOwner.Chains, 1)
	require.Equal("remote", byOwner.Chains[0].Descriptor.Name)

	resp := h.call(t, "minichain.list", map[string]interface{}{"limit": maxPageSize + 1}, nil)
	require.NotNil(resp.Error)
	require.Equal(int(CodeInvalidParams), resp.Error.Code)

	resp = h.call(t, "minichain.list", map[string]interface{}{"lifecycle": "SLEEPING"}, nil)
	require.NotNil(resp.Error)
	require.Equal("invalid_lifecycle", resp.Error.Data.Reason)
}

func TestServiceGetErrors(t *testing.T) {
	require := require.New(t)
	h := newHarness(t)

	resp := h.call(t, "minichain.get", map[string]string{"chainID": ids.GenerateTestID().String()}, nil)
	require.NotNil(resp.Error)
	require.Equal(int(CodeNotFound), resp.Error.Code)

	resp = h.call(t, "minichain.get", map[string]string{"chainID": "M"}, nil)
	require.NotNil(resp.Error)
	require.Equal(int(CodeNotMiniChain), resp.Error.Code)

	resp = h.call(t, "minichain.get", map[string]string{}, nil)
	require.NotNil(resp.Error)
	require.Equal(int(CodeInvalidParams), resp.Error.Code)

	// Internal errors never leak details.
	h.registry.failChains = true
	resp = h.call(t, "minichain.list", map[string]interface{}{}, nil)
	require.NotNil(resp.Error)
	require.Equal(int(CodeInternal), resp.Error.Code)
	require.NotContains(resp.Error.Message, "/secret/path")
}

func TestServiceValidatorsStatsHealth(t *testing.T) {
	require := require.New(t)
	h := newHarness(t)

	var vdrs ValidatorsReply
	require.Nil(h.call(t, "minichain.validators", map[string]string{"chainID": h.active.ChainID.String()}, &vdrs).Error)
	require.Len(vdrs.Members, 3)
	require.Equal(MembershipSource, vdrs.MembershipSource)
	require.Equal(SourceLocalObservation, vdrs.ConnectivitySource)
	require.True(vdrs.Summary.ConnectivityObservable)
	require.NotNil(vdrs.Summary.ConnectedWeightFraction)
	require.Equal(1.0, *vdrs.Summary.ConnectedWeightFraction)
	require.Equal(EnforcedByConsensus, vdrs.Policy.Enforcement)

	var remote ValidatorsReply
	require.Nil(h.call(t, "minichain.validators", map[string]string{"chainID": h.remote.ChainID.String()}, &remote).Error)
	require.False(remote.Summary.ConnectivityObservable)
	require.Nil(remote.Summary.ConnectedCount)
	require.Nil(remote.Members[0].Connected)

	var stats StatsReply
	require.Nil(h.call(t, "minichain.stats", map[string]string{"chainID": h.active.ChainID.String()}, &stats).Error)
	require.NotNil(stats.AcceptedTxs)
	require.Equal(uint64(3), uint64(*stats.AcceptedTxs))
	require.Equal(uint64(2), uint64(*stats.AcceptedContainers))
	require.Nil(stats.Height)
	require.Equal(UnavailableNoLinearHeight, stats.Unavailable["height"])
	require.NotNil(stats.LastAcceptedAt)
	require.NotNil(stats.LastAcceptedContainerID)
	require.NotContains(stats.Unavailable, "lastAcceptedAt")

	var remoteStats StatsReply
	require.Nil(h.call(t, "minichain.stats", map[string]string{"chainID": h.remote.ChainID.String()}, &remoteStats).Error)
	require.Nil(remoteStats.AcceptedTxs)
	require.Equal(UnavailableNotTracked, remoteStats.Unavailable["acceptedTxs"])

	var health HealthReply
	require.Nil(h.call(t, "minichain.health", map[string]string{"chainID": h.failed.ChainID.String()}, &health).Error)
	require.Equal(Failed, health.Observation.Lifecycle)
	require.False(health.Engine.Checked)
	require.True(health.Dependencies.MChainBootstrapped)
}

func TestServiceCapabilitiesAndCreationStatus(t *testing.T) {
	require := require.New(t)
	h := newHarness(t)

	var caps CapabilitiesReply
	require.Nil(h.call(t, "minichain.capabilities", map[string]string{}, &caps).Error)
	require.Equal(Methods, caps.Methods)
	require.Nil(caps.Chain)
	require.Contains(caps.SupportedVMs, constants.AVMID)

	require.Nil(h.call(t, "minichain.capabilities", map[string]string{"chainID": h.active.ChainID.String()}, &caps).Error)
	require.NotNil(caps.Chain)
	require.NotEmpty(caps.Chain.APIs)

	subnetTx := h.active.SubnetID
	h.registry.txs[subnetTx] = TxCommitted
	var status CreationStatusReply
	require.Nil(h.call(t, "minichain.getCreationStatus", map[string]string{"subnetTxID": subnetTx.String()}, &status).Error)
	require.Equal(StageSubnetRegistered, status.Stage)

	h.registry.txs[h.active.ChainID] = TxProcessing
	require.Nil(h.call(t, "minichain.getCreationStatus", map[string]string{
		"subnetTxID": subnetTx.String(), "chainTxID": h.active.ChainID.String(),
	}, &status).Error)
	require.Equal(StageChainPending, status.Stage)

	h.registry.txs[h.active.ChainID] = TxCommitted
	require.Nil(h.call(t, "minichain.getCreationStatus", map[string]string{
		"subnetTxID": subnetTx.String(), "chainTxID": h.active.ChainID.String(),
	}, &status).Error)
	require.Equal(StageRegistered, status.Stage)
	require.NotNil(status.Observation)
	require.Equal(Active, status.Observation.Lifecycle)

	// A chain registered on a different subnet is rejected.
	h.registry.txs[h.failed.SubnetID] = TxCommitted
	resp := h.call(t, "minichain.getCreationStatus", map[string]string{
		"subnetTxID": h.failed.SubnetID.String(), "chainTxID": h.active.ChainID.String(),
	}, nil)
	require.NotNil(resp.Error)
	require.Equal("subnet_mismatch", resp.Error.Data.Reason)
}

func TestObserverEventsFromPoll(t *testing.T) {
	require := require.New(t)
	h := newHarness(t)

	var events EventsReply
	require.Nil(h.call(t, "minichain.events", map[string]interface{}{}, &events).Error)
	// Each tracked chain is first observed, followed by the event for the
	// state it was first observed in.
	firstTypes := map[ids.ID][]EventType{}
	for _, e := range events.Events {
		firstTypes[e.ChainID] = append(firstTypes[e.ChainID], e.Type)
		require.False(e.Authoritative)
		require.Equal(h.observer.BootID(), e.BootID)
	}
	require.Equal([]EventType{EventRegisteredObserved, EventActivated}, firstTypes[h.active.ChainID])
	require.Equal([]EventType{EventRegisteredObserved, EventFailed}, firstTypes[h.failed.ChainID])
	require.NotContains(firstTypes, h.remote.ChainID, "untracked chains produce no local events")
	require.Equal(3, h.observer.Registered())

	// The failed chain is retried and starts bootstrapping: a valid
	// FAILED -> PENDING -> INITIALISING sequence is recorded.
	h.local[h.failed.ChainID] = chains.LocalChainStatus{ChainID: h.failed.ChainID}
	h.observer.Poll(context.Background())
	h.local[h.failed.ChainID] = chains.LocalChainStatus{ChainID: h.failed.ChainID, Created: true, State: snow.Bootstrapping}
	h.observer.Poll(context.Background())

	// Committee membership change of the active chain.
	h.registry.committees[h.active.SubnetID] = connectedMembers(4, true)
	h.observer.Poll(context.Background())

	var next EventsReply
	require.Nil(h.call(t, "minichain.events", map[string]interface{}{"after": events.NextCursor}, &next).Error)
	types := []EventType{}
	for _, e := range next.Events {
		types = append(types, e.Type)
	}
	require.Equal([]EventType{EventLifecycleChanged, EventInitialisationStarted, EventValidatorMembershipChanged}, types)
	require.Equal(Failed, next.Events[0].From)
	require.Equal(Pending, next.Events[0].To)
}
