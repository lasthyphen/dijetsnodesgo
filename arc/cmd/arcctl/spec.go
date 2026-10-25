package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/arc/workflow"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// Spec is the file Arc Studio exports (arcspec 1). Amounts are in base
// units; addresses are bech32 with or without a chain prefix. The offchain
// block is Arc Studio's own and is never sent to the network.
type Spec struct {
	ArcSpec   int    `json:"arcspec"`
	Name      string `json:"name"`
	NetworkID uint32 `json:"networkID"`
	VM        string `json:"vm"`
	Genesis   struct {
		Assets map[string]avm.AssetDefinition `json:"assets"`
	} `json:"genesis"`
	Subnet struct {
		Owner *struct {
			Threshold uint32   `json:"threshold"`
			Addresses []string `json:"addresses"`
		} `json:"owner,omitempty"`
		SubnetID string `json:"subnetID,omitempty"`
	} `json:"subnet"`
	Committee struct {
		Members []struct {
			NodeID string `json:"nodeID"`
			Weight uint64 `json:"weight"`
		} `json:"members"`
		StartDelay string `json:"startDelay"`
		Duration   string `json:"duration"`
	} `json:"committee"`
	Offchain json.RawMessage `json:"offchain,omitempty"`
}

var (
	errSpecVersion = errors.New("unsupported spec: arcspec must be 1")
	errSpecVM      = errors.New("unsupported spec: vm must be avm")
	errWrongHRP    = errors.New("address is for a different network")
)

func readSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &Spec{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("couldn't parse %s: %w", path, err)
	}
	return s, nil
}

// request turns a spec into the workflow's request, with the committee
// starting [StartDelay] after [now].
func (s *Spec) request(now time.Time) (workflow.Request, error) {
	req := workflow.Request{Name: s.Name}
	switch {
	case s.ArcSpec != 1:
		return req, errSpecVersion
	case s.VM != "avm":
		return req, errSpecVM
	}
	genesis, err := workflow.BuildAVMGenesis(s.NetworkID, s.Genesis.Assets)
	if err != nil {
		return req, fmt.Errorf("couldn't build genesis: %w", err)
	}
	req.Genesis = genesis

	hrp := constants.GetHRP(s.NetworkID)
	if s.Subnet.SubnetID != "" {
		if req.SubnetID, err = ids.FromString(s.Subnet.SubnetID); err != nil {
			return req, fmt.Errorf("invalid subnetID: %w", err)
		}
	}
	if o := s.Subnet.Owner; o != nil {
		owner := &secp256k1fx.OutputOwners{Threshold: o.Threshold}
		for _, a := range o.Addresses {
			id, err := parseAddress(a, hrp)
			if err != nil {
				return req, fmt.Errorf("owner address %q: %w", a, err)
			}
			owner.Addrs = append(owner.Addrs, id)
		}
		owner.Sort()
		req.Owner = owner
	}
	for _, m := range s.Committee.Members {
		nodeID, err := ids.NodeIDFromString(m.NodeID)
		if err != nil {
			return req, fmt.Errorf("committee member %q: %w", m.NodeID, err)
		}
		req.Committee = append(req.Committee, workflow.Member{NodeID: nodeID, Weight: m.Weight})
	}
	delay, err := time.ParseDuration(s.Committee.StartDelay)
	if err != nil {
		return req, fmt.Errorf("invalid startDelay: %w", err)
	}
	dur, err := time.ParseDuration(s.Committee.Duration)
	if err != nil {
		return req, fmt.Errorf("invalid duration: %w", err)
	}
	req.ValidationStart = now.Add(delay).Truncate(time.Second)
	req.ValidationEnd = req.ValidationStart.Add(dur)
	return req, nil
}

// parseAddress accepts "M-local1…" or "local1…" on network [hrp].
func parseAddress(a, hrp string) (ids.ShortID, error) {
	bare := a
	if i := strings.IndexByte(a, '-'); i > 0 {
		bare = a[i+1:]
	}
	got, b, err := address.ParseBech32(bare)
	if err != nil {
		return ids.ShortEmpty, err
	}
	if got != hrp {
		return ids.ShortEmpty, fmt.Errorf("%w: %s, not %s", errWrongHRP, got, hrp)
	}
	return ids.ToShortID(b)
}
