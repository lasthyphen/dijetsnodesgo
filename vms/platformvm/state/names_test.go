package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

type singleVersion struct {
	blkID ids.ID
	state Chain
}

func (v singleVersion) GetState(blkID ids.ID) (Chain, bool) {
	return v.state, blkID == v.blkID
}

func testNameRecord(name string, target ids.ShortID, version uint64) *names.Record {
	return &names.Record{
		Name:   name,
		Target: target,
		Owner: &secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs:     []ids.ShortID{target},
		},
		Expiry:     1000,
		Generation: 1,
		Version:    version,
		RecordID:   ids.ID{1},
		LastTxID:   ids.ID{byte(version)},
	}
}

// Registry state written through a diff survives a restart of the node.
func TestNamesPersistAcrossRestart(t *testing.T) {
	require := require.New(t)
	s, db := newInitializedState(require)

	alice := ids.ShortID{'a'}
	bob := ids.ShortID{'b'}
	blkID := ids.GenerateTestID()
	diff, err := NewDiff(blkID, singleVersion{blkID: blkID, state: s})
	require.NoError(err)

	// Two versions of one record within a single diff, as two updates in one
	// block would produce.
	diff.PutName(testNameRecord("alice", alice, 1))
	diff.PutNameTarget(alice, "alice")
	diff.PutName(testNameRecord("alice", bob, 2))
	diff.DeleteNameTarget(alice)
	diff.PutNameTarget(bob, "alice")

	// The diff sees its own changes; the parent doesn't yet.
	record, err := diff.GetName("alice")
	require.NoError(err)
	require.Equal(uint64(2), record.Version)
	_, err = diff.GetNameTarget(alice)
	require.ErrorIs(err, database.ErrNotFound)
	_, err = s.GetName("alice")
	require.ErrorIs(err, database.ErrNotFound)

	diff.Apply(s)
	require.NoError(s.Commit())
	require.NoError(s.Close())

	restarted := newStateFromDB(require, db)
	record, err = restarted.GetName("alice")
	require.NoError(err)
	require.Equal(testNameRecord("alice", bob, 2), record)

	name, err := restarted.GetNameTarget(bob)
	require.NoError(err)
	require.Equal("alice", name)
	_, err = restarted.GetNameTarget(alice)
	require.ErrorIs(err, database.ErrNotFound)

	history, err := restarted.GetNameHistory("alice")
	require.NoError(err)
	require.Equal([]*names.Record{
		testNameRecord("alice", alice, 1),
		testNameRecord("alice", bob, 2),
	}, history)

	// History lookups don't leak across names sharing a prefix.
	history, err = restarted.GetNameHistory("ali")
	require.NoError(err)
	require.Empty(history)
}
