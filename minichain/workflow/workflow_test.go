package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/minichain"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

func testGenesis(t *testing.T, memo string) []byte {
	addr, err := address.FormatBech32(constants.GetHRP(constants.UnitTestID), ids.GenerateTestShortID().Bytes())
	require.NoError(t, err)
	genesis, err := BuildAVMGenesis(constants.UnitTestID, map[string]avm.AssetDefinition{
		"MINI": {
			Name:         "Mini Token",
			Symbol:       "MINI",
			Denomination: 9,
			Memo:         memo,
			InitialState: map[string][]interface{}{
				"fixedCap": {avm.Holder{Amount: 1_000_000, Address: addr}},
			},
		},
	})
	require.NoError(t, err)
	return genesis
}

func TestAVMGenesisValidation(t *testing.T) {
	require := require.New(t)

	a := testGenesis(t, "0x01")
	b := testGenesis(t, "0x02")
	require.NoError(ValidateAVMGenesis(a))
	require.NoError(ValidateAVMGenesis(b))
	require.NotEqual(minichain.GenesisHash(a), minichain.GenesisHash(b), "distinct memos give distinct genesis")

	require.Error(ValidateAVMGenesis([]byte("not a genesis")))
	require.Error(ValidateAVMGenesis(nil))
	require.Error(ValidateAVMGenesis(a[:len(a)-3]))

	empty, err := BuildAVMGenesis(constants.UnitTestID, map[string]avm.AssetDefinition{})
	require.NoError(err)
	require.ErrorIs(ValidateAVMGenesis(empty), ErrGenesisNoAssets)
}

func validRequest(t *testing.T) Request {
	now := time.Now()
	return Request{
		Name:    "alpha",
		Genesis: testGenesis(t, "0x01"),
		Owner: &secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
		},
		Committee: []Member{
			{NodeID: ids.GenerateTestNodeID(), Weight: 20},
			{NodeID: ids.GenerateTestNodeID(), Weight: 20},
			{NodeID: ids.GenerateTestNodeID(), Weight: 20},
		},
		ValidationStart: now.Add(time.Minute),
		ValidationEnd:   now.Add(48 * time.Hour),
	}
}

func TestValidateRequest(t *testing.T) {
	require.NoError(t, ValidateRequest(validRequest(t)))

	tests := []struct {
		name   string
		mutate func(*Request)
		err    error
	}{
		{"bad name", func(r *Request) { r.Name = "bad-name" }, nil},
		{"bad genesis", func(r *Request) { r.Genesis = []byte{1, 2, 3} }, nil},
		{"missing owner", func(r *Request) { r.Owner = nil }, ErrMissingOwner},
		{"invalid owner", func(r *Request) { r.Owner.Threshold = 2 }, nil},
		{"empty period", func(r *Request) { r.ValidationEnd = r.ValidationStart }, ErrInvalidPeriod},
		{"committee too small", func(r *Request) { r.Committee = r.Committee[:2] }, ErrCommitteeTooSmall},
		{"duplicate member", func(r *Request) { r.Committee[1].NodeID = r.Committee[0].NodeID }, ErrDuplicateMember},
		{"zero weight", func(r *Request) { r.Committee[2].Weight = 0 }, ErrZeroWeight},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := validRequest(t)
			test.mutate(&req)
			err := ValidateRequest(req)
			require.Error(t, err)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			}
		})
	}

	// Reusing a subnet does not need an owner.
	req := validRequest(t)
	req.Owner = nil
	req.SubnetID = ids.GenerateTestID()
	require.NoError(t, ValidateRequest(req))

	// The minimum committee size can be lowered explicitly.
	req = validRequest(t)
	req.Committee = req.Committee[:1]
	req.MinCommitteeSize = 1
	require.NoError(t, ValidateRequest(req))
}
