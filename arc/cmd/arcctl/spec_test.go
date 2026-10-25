package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/arc/workflow"
	"github.com/lasthyphen/dijetsnodesgo/genesis"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
)

// An Arc Studio export for the local network, as StageReview writes it.
func studioSpec(t *testing.T) []byte {
	hrp := constants.GetHRP(constants.LocalID)
	addr, err := address.FormatBech32(hrp, genesis.EWOQKey.PublicKey().Address().Bytes())
	require.NoError(t, err)
	return []byte(`{
  "arcspec": 1,
  "name": "Household Ledger",
  "networkID": 12345,
  "vm": "avm",
  "genesis": {"assets": {"credit": {"name": "Household Credit", "symbol": "HHC", "denomination": 2,
    "initialState": {"fixedCap": [{"amount": "2500000", "address": "` + addr + `"}]}}}},
  "subnet": {"owner": {"threshold": 1, "addresses": ["M-` + addr + `"]}},
  "committee": {"members": [
    {"nodeID": "NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg", "weight": 20},
    {"nodeID": "NodeID-MFrZFVCXPv5iCn6M9K6XduxGTYp891xXZ", "weight": 20},
    {"nodeID": "NodeID-NFBbbJ4qCmNaCzeW7sxErhvWqvEQMnYcN", "weight": 20}],
    "startDelay": "10m", "duration": "720h"},
  "offchain": {"template": "finance", "purpose": "Personal finance assistant"}
}`)
}

func TestStudioSpecBuildsAValidRequest(t *testing.T) {
	s := &Spec{}
	require.NoError(t, json.Unmarshal(studioSpec(t), s))
	now := time.Unix(1_800_000_000, 0)
	req, err := s.request(now)
	require.NoError(t, err)
	require.NoError(t, workflow.ValidateRequest(req))
	require.Equal(t, "Household Ledger", req.Name)
	require.Len(t, req.Committee, 3)
	require.Equal(t, now.Add(10*time.Minute), req.ValidationStart)
	require.Equal(t, 720*time.Hour, req.ValidationEnd.Sub(req.ValidationStart))
	require.Equal(t, uint32(1), req.Owner.Threshold)
	require.Equal(t, genesis.EWOQKey.PublicKey().Address(), req.Owner.Addrs[0])
}

func TestSpecRejectsAnotherNetworksAddress(t *testing.T) {
	s := &Spec{}
	require.NoError(t, json.Unmarshal(studioSpec(t), s))
	mainnet, err := address.FormatBech32(constants.GetHRP(constants.MainnetID), genesis.EWOQKey.PublicKey().Address().Bytes())
	require.NoError(t, err)
	s.Subnet.Owner.Addresses = []string{"M-" + mainnet}
	_, err = s.request(time.Now())
	require.ErrorIs(t, err, errWrongHRP)
}

func TestSpecRejectsNonAVM(t *testing.T) {
	s := &Spec{}
	require.NoError(t, json.Unmarshal(studioSpec(t), s))
	s.VM = "evm"
	_, err := s.request(time.Now())
	require.ErrorIs(t, err, errSpecVM)
}
