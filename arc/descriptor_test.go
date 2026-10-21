package arc

import (
	stdjson "encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

func TestValidateName(t *testing.T) {
	valid := []string{"a", "Alpha", "alpha 1", "Personal Network 42", strings.Repeat("a", MaxNameLen)}
	for _, name := range valid {
		require.NoError(t, ValidateName(name), name)
	}
	invalid := []string{"", " alpha", "1alpha", "alpha ", "al  pha", "alpha-1", "alpha_1", "αlpha", strings.Repeat("a", MaxNameLen+1)}
	for _, name := range invalid {
		require.Error(t, ValidateName(name), name)
	}
}

func TestValidateGenesisEnvelope(t *testing.T) {
	require := require.New(t)
	require.NoError(ValidateGenesisEnvelope(constants.AVMID, []byte{1}))
	require.ErrorIs(ValidateGenesisEnvelope(constants.EVMID, []byte{1}), errUnsupportedVM)
	require.ErrorIs(ValidateGenesisEnvelope(constants.AVMID, nil), errEmptyGenesis)
	require.ErrorIs(ValidateGenesisEnvelope(constants.AVMID, make([]byte, maxGenesisBytes+1)), errGenesisTooLarge)
}

func TestGenesisHashDeterministicAndDistinct(t *testing.T) {
	require := require.New(t)
	a := GenesisHash([]byte("genesis a"))
	require.Equal(a, GenesisHash([]byte("genesis a")))
	require.NotEqual(a, GenesisHash([]byte("genesis b")))
	require.True(strings.HasPrefix(FormatGenesisHash(a), "sha256:"))
	require.Len(FormatGenesisHash(a), len("sha256:")+64)
}

func testRecord() Record {
	return Record{
		ChainID:          ids.GenerateTestID(),
		SubnetID:         ids.GenerateTestID(),
		Name:             "alpha",
		VMID:             constants.AVMID,
		FxIDs:            []ids.ID{secp256k1fx.ID},
		GenesisHash:      GenesisHash([]byte("g")),
		GenesisSize:      1,
		Owner:            Owner{Threshold: 1, Addresses: []string{"M-local1abc"}},
		SubnetChainCount: 1,
	}
}

func TestBuildDescriptor(t *testing.T) {
	require := require.New(t)
	rec := testRecord()
	eval := EvaluateCommittee(PermissionedCommittee, true, nil, DefaultPolicyParams())

	d := BuildDescriptor(rec, 7, 1337, eval, DefaultPolicyParams(), false)
	require.Equal(SchemaVersion, d.SchemaVersion)
	require.Equal(ProtocolVersion, d.ProtocolVersion)
	require.Equal(rec.ChainID, d.ID)
	require.Equal(rec.ChainID, d.CreatedBy.ChainTxID)
	require.Equal(rec.SubnetID, d.CreatedBy.SubnetTxID)
	require.Equal("avm", d.VMName)
	require.Nil(d.CreatedAt)
	require.Equal(UnavailableNotIndexed, d.Unavailable["createdAt"])
	require.Equal(UnavailableNotServedLocally, d.Unavailable["rpcEndpoints"])
	require.Empty(d.RPCEndpoints)
	require.Equal(PrivacyPublic, d.PrivacyMode)
	require.Equal(uint64(7), d.Registration.ObservedAtMChainHeight)
	require.Contains(d.Capabilities, "consensus:dag")
	require.Contains(d.Capabilities, "fx:secp256k1fx")

	served := BuildDescriptor(rec, 7, 1337, eval, DefaultPolicyParams(), true)
	require.Equal([]string{"/ext/bc/" + rec.ChainID.String()}, served.RPCEndpoints)
	require.NotContains(served.Unavailable, "rpcEndpoints")
}

func TestDescriptorJSONRoundTrip(t *testing.T) {
	require := require.New(t)
	rec := testRecord()
	eval := EvaluateCommittee(PermissionedCommittee, true, nil, DefaultPolicyParams())
	d := BuildDescriptor(rec, 1, 1337, eval, DefaultPolicyParams(), true)

	b, err := stdjson.Marshal(d)
	require.NoError(err)
	for _, field := range []string{
		`"schemaVersion":1`, `"protocolVersion":"arc/1"`, `"metadataVersion":1`,
		`"createdAt":null`, `"explorerURL":null`, `"privacyMode":"public"`,
	} {
		require.Contains(string(b), field)
	}

	var decoded Descriptor
	require.NoError(stdjson.Unmarshal(b, &decoded))
	require.Equal(d, decoded)
}

func TestDescriptorNeverContainsSecrets(t *testing.T) {
	b, err := stdjson.Marshal(BuildDescriptor(testRecord(), 1, 1, CommitteeEvaluation{}, DefaultPolicyParams(), true))
	require.NoError(t, err)
	lower := strings.ToLower(string(b))
	for _, forbidden := range []string{"privatekey", "private_key", "secret", "password", "signature", "credential"} {
		require.NotContains(t, lower, forbidden)
	}
}
