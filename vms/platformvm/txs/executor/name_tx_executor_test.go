package executor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/version"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/state"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// nameHarness executes name transactions one after another against a real,
// committed platformvm state.
type nameHarness struct {
	t   *testing.T
	env *environment
	now time.Time
}

func newNameHarness(t *testing.T) *nameHarness {
	env := newEnvironment(true /*=postBanff*/)
	env.config.NamingParams = names.DevelopmentParams
	env.ctx.Lock.Lock()
	t.Cleanup(func() {
		require.NoError(t, shutdownEnvironment(env))
	})
	return &nameHarness{
		t:   t,
		env: env,
		now: defaultValidateEndTime,
	}
}

func (h *nameHarness) unix(d time.Duration) uint64 {
	return uint64(h.now.Add(d).Unix())
}

// sign builds the base tx inputs paying [fee] from the first pre-funded key
// and signs [utx], appending one credential per entry of [authKeys].
func (h *nameHarness) sign(utx txs.UnsignedTx, fee uint64, authKeys ...[]*crypto.PrivateKeySECP256K1R) *txs.Tx {
	return h.signFrom(preFundedKeys[:1], utx, fee, authKeys...)
}

// signFrom is sign, paying [fee] from [payers].
func (h *nameHarness) signFrom(payers []*crypto.PrivateKeySECP256K1R, utx txs.UnsignedTx, fee uint64, authKeys ...[]*crypto.PrivateKeySECP256K1R) *txs.Tx {
	ins, outs, _, signers, err := h.env.utxosHandler.Spend(payers, 0, fee, preFundedKeys[0].PublicKey().Address())
	require.NoError(h.t, err)

	baseTx := txs.BaseTx{BaseTx: djtx.BaseTx{
		NetworkID:    h.env.ctx.NetworkID,
		BlockchainID: h.env.ctx.ChainID,
		Ins:          ins,
		Outs:         outs,
	}}
	switch utx := utx.(type) {
	case *txs.RegisterNameTx:
		utx.BaseTx = baseTx
	case *txs.UpdateNameTx:
		utx.BaseTx = baseTx
	}
	tx := &txs.Tx{Unsigned: utx}
	require.NoError(h.t, tx.Sign(txs.Codec, append(signers, authKeys...)))
	return tx
}

// execute verifies [tx] at the harness time and, if it is valid, commits its
// changes.
func (h *nameHarness) execute(tx *txs.Tx) error {
	diff, err := state.NewDiff(lastAcceptedID, h.env)
	require.NoError(h.t, err)
	diff.SetTimestamp(h.now)

	if err := tx.Unsigned.Visit(&StandardTxExecutor{
		Backend: &h.env.backend,
		State:   diff,
		Tx:      tx,
	}); err != nil {
		return err
	}
	diff.Apply(h.env.state)
	require.NoError(h.t, h.env.state.Commit())
	return nil
}

func owner(keys ...*crypto.PrivateKeySECP256K1R) *secp256k1fx.OutputOwners {
	o := &secp256k1fx.OutputOwners{Threshold: 1}
	for _, key := range keys {
		o.Addrs = append(o.Addrs, key.PublicKey().Address())
	}
	return o
}

func keys(k ...*crypto.PrivateKeySECP256K1R) []*crypto.PrivateKeySECP256K1R {
	return k
}

var (
	alice = preFundedKeys[1]
	bob   = preFundedKeys[2]
	carol = preFundedKeys[3]
)

// register registers [name] for [target], owned by [target], signed by
// [signer].
func (h *nameHarness) register(name string, target, signer *crypto.PrivateKeySECP256K1R, expiry uint64) *txs.Tx {
	return h.sign(&txs.RegisterNameTx{
		Name:       name,
		Target:     target.PublicKey().Address(),
		Owner:      owner(target),
		Expiry:     expiry,
		TargetAuth: &secp256k1fx.Input{SigIndices: []uint32{0}},
	}, names.DevelopmentParams.RegisterFee, keys(signer))
}

// update builds an UpdateNameTx of [current] signed by [ownerKey]. If the
// target changes, [targetKey] signs for the new target.
func (h *nameHarness) update(
	current *names.Record,
	ownerKey *crypto.PrivateKeySECP256K1R,
	target *crypto.PrivateKeySECP256K1R,
	newOwner *secp256k1fx.OutputOwners,
	expiry uint64,
	revoke bool,
	targetKey *crypto.PrivateKeySECP256K1R,
) *txs.Tx {
	targetAuth := &secp256k1fx.Input{}
	targetKeys := keys()
	if targetKey != nil {
		targetAuth.SigIndices = []uint32{0}
		targetKeys = keys(targetKey)
	}
	return h.sign(&txs.UpdateNameTx{
		Name:       current.Name,
		Version:    current.Version,
		Target:     target.PublicKey().Address(),
		Owner:      newOwner,
		Expiry:     expiry,
		Revoke:     revoke,
		NameAuth:   &secp256k1fx.Input{SigIndices: []uint32{0}},
		TargetAuth: targetAuth,
	}, defaultTxFee, keys(ownerKey), targetKeys)
}

func (h *nameHarness) record(name string) *names.Record {
	record, err := h.env.state.GetName(name)
	require.NoError(h.t, err)
	return record
}

func (h *nameHarness) status(name string) names.Status {
	record, err := h.env.state.GetName(name)
	if err == database.ErrNotFound {
		return names.StatusNotFound
	}
	require.NoError(h.t, err)
	return record.StatusAt(h.now, names.DevelopmentParams.GracePeriod)
}

func (h *nameHarness) targetOf(key *crypto.PrivateKeySECP256K1R) string {
	name, err := h.env.state.GetNameTarget(key.PublicKey().Address())
	if err == database.ErrNotFound {
		return ""
	}
	require.NoError(h.t, err)
	return name
}

func TestRegisterName(t *testing.T) {
	require := require.New(t)
	h := newNameHarness(t)

	tx := h.register("alice", alice, alice, h.unix(time.Hour))
	require.NoError(h.execute(tx))

	record := h.record("alice")
	require.Equal("alice", record.Name)
	require.Equal(alice.PublicKey().Address(), record.Target)
	require.Equal(uint64(1), record.Generation)
	require.Equal(uint64(1), record.Version)
	require.Equal(tx.ID(), record.RecordID)
	require.Equal(tx.ID(), record.LastTxID)
	require.Equal(uint64(h.now.Unix()), record.RegisteredAt)
	require.Equal(names.StatusActive, h.status("alice"))
	require.Equal("alice", h.targetOf(alice))

	history, err := h.env.state.GetNameHistory("alice")
	require.NoError(err)
	require.Len(history, 1)
}

func TestRegisterNameRejections(t *testing.T) {
	h := newNameHarness(t)
	require.NoError(t, h.execute(h.register("alice", alice, alice, h.unix(time.Hour))))

	tests := []struct {
		name string
		tx   func() *txs.Tx
		err  error
	}{
		{
			name: "duplicate name",
			tx:   func() *txs.Tx { return h.register("alice", bob, bob, h.unix(time.Hour)) },
			err:  ErrNameNotRegistrable,
		},
		{
			name: "target already bound",
			tx:   func() *txs.Tx { return h.register("alice2", alice, alice, h.unix(time.Hour)) },
			err:  ErrNameTargetTaken,
		},
		{
			name: "target did not consent",
			tx:   func() *txs.Tx { return h.register("bob", bob, carol, h.unix(time.Hour)) },
			err:  ErrUnauthorizedNameTarget,
		},
		{
			name: "expiry shorter than the minimum term",
			tx:   func() *txs.Tx { return h.register("bob", bob, bob, h.unix(30*time.Second)) },
			err:  ErrNameExpiryOutOfRange,
		},
		{
			name: "expiry beyond the maximum term",
			tx: func() *txs.Tx {
				return h.register("bob", bob, bob, h.unix(names.DevelopmentParams.MaxTerm+time.Second))
			},
			err: ErrNameExpiryOutOfRange,
		},
		{
			name: "non-canonical name",
			tx:   func() *txs.Tx { return h.register("Bob", bob, bob, h.unix(time.Hour)) },
			err:  names.ErrInvalidName,
		},
		{
			name: "reserved name",
			tx:   func() *txs.Tx { return h.register("admin", bob, bob, h.unix(time.Hour)) },
			err:  names.ErrInvalidName,
		},
		{
			name: "malformed name",
			tx:   func() *txs.Tx { return h.register("b", bob, bob, h.unix(time.Hour)) },
			err:  names.ErrInvalidName,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, h.execute(test.tx()), test.err)
		})
	}

	// The failed attempts left the registry untouched.
	require.Equal(t, "alice", h.targetOf(alice))
	require.Equal(t, "", h.targetOf(bob))
	require.Equal(t, uint64(1), h.record("alice").Version)
}

func TestRegisterNameFee(t *testing.T) {
	h := newNameHarness(t)
	tx := h.sign(&txs.RegisterNameTx{
		Name:       "alice",
		Target:     alice.PublicKey().Address(),
		Owner:      owner(alice),
		Expiry:     h.unix(time.Hour),
		TargetAuth: &secp256k1fx.Input{SigIndices: []uint32{0}},
	}, names.DevelopmentParams.RegisterFee-1, keys(alice))
	require.Error(t, h.execute(tx))
}

func TestNamingNotActivated(t *testing.T) {
	h := newNameHarness(t)
	h.env.config.NamingTime = h.now.Add(time.Second)

	err := h.execute(h.register("alice", alice, alice, h.unix(time.Hour)))
	require.ErrorIs(t, err, ErrNamingNotActivated)
}

// Mainnet runs Banff and naming from genesis, with the public registry
// parameters. Lothar does not schedule naming.
func TestMainnetNaming(t *testing.T) {
	require := require.New(t)
	h := newNameHarness(t)
	h.now = time.Unix(1792916856, 0) // the mainnet genesis start time
	h.env.config.BanffTime = version.GetBanffTime(constants.MainnetID)
	h.env.config.NamingTime = version.GetNamingTime(constants.MainnetID)
	h.env.config.NamingParams = names.GetParams(constants.MainnetID)
	require.Equal(names.PublicParams, h.env.config.NamingParams)
	require.True(h.env.config.IsNamingActivated(h.now))

	// One pre-funded key holds less than the mainnet fee; pay from all of them.
	register := func(name string, term time.Duration, fee uint64) *txs.Tx {
		return h.signFrom(preFundedKeys, &txs.RegisterNameTx{
			Name:       name,
			Target:     alice.PublicKey().Address(),
			Owner:      owner(alice),
			Expiry:     h.unix(term),
			TargetAuth: &secp256k1fx.Input{SigIndices: []uint32{0}},
		}, fee, keys(alice))
	}
	year := 365 * 24 * time.Hour
	// The development fee and a term under 30 days are refused on mainnet.
	require.Error(h.execute(register("alice", year, names.DevelopmentParams.RegisterFee)))
	require.Error(h.execute(register("alice", time.Hour, names.PublicParams.RegisterFee)))
	require.NoError(h.execute(register("alice", year, names.PublicParams.RegisterFee)))
	require.Equal(names.StatusActive, h.status("alice"))

	require.True(version.GetNamingTime(constants.LotharID).After(h.now))
}

func TestUpdateNameTarget(t *testing.T) {
	require := require.New(t)
	h := newNameHarness(t)
	expiry := h.unix(time.Hour)
	require.NoError(h.execute(h.register("alice", alice, alice, expiry)))
	v1 := h.record("alice")

	// Moving the name needs the new target's consent.
	err := h.execute(h.update(v1, alice, bob, owner(alice), expiry, false, carol))
	require.ErrorIs(err, ErrUnauthorizedNameTarget)

	tx := h.update(v1, alice, bob, owner(alice), expiry, false, bob)
	require.NoError(h.execute(tx))
	v2 := h.record("alice")
	require.Equal(bob.PublicKey().Address(), v2.Target)
	require.Equal(uint64(2), v2.Version)
	require.Equal(v1.RecordID, v2.RecordID)
	require.Equal(tx.ID(), v2.LastTxID)
	require.Equal("", h.targetOf(alice))
	require.Equal("alice", h.targetOf(bob))

	// An update prepared against version 1 is stale now.
	err = h.execute(h.update(v1, alice, alice, owner(alice), expiry, false, alice))
	require.ErrorIs(err, ErrNameVersionMismatch)

	// With the target unchanged, the target authorization must be empty.
	err = h.execute(h.update(v2, alice, bob, owner(alice), expiry, false, bob))
	require.ErrorIs(err, ErrUnexpectedNameTargetAuth)

	history, err := h.env.state.GetNameHistory("alice")
	require.NoError(err)
	require.Len(history, 2)
	require.Equal(alice.PublicKey().Address(), history[0].Target)
	require.Equal(bob.PublicKey().Address(), history[1].Target)
}

func TestUpdateNameAuthorization(t *testing.T) {
	require := require.New(t)
	h := newNameHarness(t)
	expiry := h.unix(time.Hour)
	require.NoError(h.execute(h.register("alice", alice, alice, expiry)))

	// Only the owner may update.
	err := h.execute(h.update(h.record("alice"), carol, alice, owner(carol), expiry, false, nil))
	require.ErrorIs(err, ErrUnauthorizedNameUpdate)

	// Transfer ownership to carol.
	require.NoError(h.execute(h.update(h.record("alice"), alice, alice, owner(carol), expiry, false, nil)))

	// The previous owner has lost control; the new owner has it.
	err = h.execute(h.update(h.record("alice"), alice, alice, owner(alice), expiry, false, nil))
	require.ErrorIs(err, ErrUnauthorizedNameUpdate)
	require.NoError(h.execute(h.update(h.record("alice"), carol, alice, owner(carol), h.unix(2*time.Hour), false, nil)))
	require.Equal(uint64(3), h.record("alice").Version)

	err = h.execute(h.update(&names.Record{Name: "nobody", Version: 1, Owner: owner(carol)}, carol, carol, owner(carol), expiry, false, nil))
	require.ErrorIs(err, ErrNameNotFound)
}

func TestNameExpiryAndRelease(t *testing.T) {
	require := require.New(t)
	h := newNameHarness(t)
	grace := names.DevelopmentParams.GracePeriod
	require.NoError(h.execute(h.register("alice", alice, alice, h.unix(time.Minute))))

	// Expired, within the grace period: no longer resolvable, but neither
	// registrable by others nor its target.
	h.now = h.now.Add(time.Minute)
	require.Equal(names.StatusExpired, h.status("alice"))
	require.ErrorIs(h.execute(h.register("alice", bob, bob, h.unix(time.Hour))), ErrNameNotRegistrable)
	require.ErrorIs(h.execute(h.register("alice2", alice, alice, h.unix(time.Hour))), ErrNameTargetTaken)

	// The owner may renew during the grace period.
	require.NoError(h.execute(h.update(h.record("alice"), alice, alice, owner(alice), h.unix(time.Minute), false, nil)))
	require.Equal(names.StatusActive, h.status("alice"))

	// After expiry and grace, the name is released.
	h.now = h.now.Add(time.Minute + grace)
	require.Equal(names.StatusReleased, h.status("alice"))
	err := h.execute(h.update(h.record("alice"), alice, alice, owner(alice), h.unix(time.Hour), false, nil))
	require.ErrorIs(err, ErrNameNotUpdatable)

	// Anyone may register it again: a new generation with a new record ID.
	previous := h.record("alice")
	tx := h.register("alice", bob, bob, h.unix(time.Hour))
	require.NoError(h.execute(tx))
	current := h.record("alice")
	require.Equal(uint64(2), current.Generation)
	require.Equal(uint64(1), current.Version)
	require.Equal(tx.ID(), current.RecordID)
	require.NotEqual(previous.RecordID, current.RecordID)
	require.Equal("", h.targetOf(alice))
	require.Equal("alice", h.targetOf(bob))

	history, err := h.env.state.GetNameHistory("alice")
	require.NoError(err)
	require.Len(history, 3)
}

func TestRevokeName(t *testing.T) {
	require := require.New(t)
	h := newNameHarness(t)
	expiry := h.unix(time.Hour)
	require.NoError(h.execute(h.register("alice", alice, alice, expiry)))

	// A revocation can't smuggle in other changes.
	err := h.execute(h.update(h.record("alice"), alice, alice, owner(alice), expiry+1, true, nil))
	require.ErrorIs(err, ErrRevokeChangesRecord)
	err = h.execute(h.update(h.record("alice"), alice, alice, owner(carol), expiry, true, nil))
	require.ErrorIs(err, ErrRevokeChangesRecord)

	require.NoError(h.execute(h.update(h.record("alice"), alice, alice, owner(alice), expiry, true, nil)))
	require.Equal(names.StatusRevoked, h.status("alice"))
	require.False(h.status("alice").Resolvable())

	// Revoked names can't be updated, and neither the name nor its target
	// can be taken during the grace period.
	err = h.execute(h.update(h.record("alice"), alice, alice, owner(alice), expiry, false, nil))
	require.ErrorIs(err, ErrNameNotUpdatable)
	require.ErrorIs(h.execute(h.register("alice", bob, bob, h.unix(time.Hour))), ErrNameNotRegistrable)
	require.ErrorIs(h.execute(h.register("alice2", alice, alice, h.unix(time.Hour))), ErrNameTargetTaken)

	h.now = h.now.Add(names.DevelopmentParams.GracePeriod)
	require.Equal(names.StatusReleased, h.status("alice"))
	require.NoError(h.execute(h.register("alice2", alice, alice, h.unix(time.Hour))))
}
