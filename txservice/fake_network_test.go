package txservice

import (
	"context"
	"errors"
	"sync"

	"github.com/gorilla/rpc/v2/json2"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/x"
)

const (
	testNetworkID = 12345
	testHRP       = "local"
	testFee       = 1_000_000 // 0.001 DJT
)

var (
	testChainID = ids.ID{'v'}
	testAssetID = ids.ID{'d', 'j', 't'}
	testAsset   = Asset{ID: testAssetID, Symbol: "DJT", Denomination: 9}
)

// fakeNetwork is an in-memory V-chain and naming registry. It parses and
// applies the transactions it is given, rejecting any that spend a missing
// UTXO, as the real chain would.
type fakeNetwork struct {
	lock     sync.Mutex
	utxos    map[ids.ID]*djtx.UTXO
	txs      map[ids.ID]*txs.Tx
	statuses map[ids.ID]choices.Status
	issued   int
	// hold keeps issued transactions Processing.
	hold bool
	down bool

	records map[string][]*platformvm.NameRecord
	expired map[string]bool
}

func newFakeNetwork() *fakeNetwork {
	return &fakeNetwork{
		utxos:    make(map[ids.ID]*djtx.UTXO),
		txs:      make(map[ids.ID]*txs.Tx),
		statuses: make(map[ids.ID]choices.Status),
		records:  make(map[string][]*platformvm.NameRecord),
		expired:  make(map[string]bool),
	}
}

func (*fakeNetwork) Context() x.Context {
	return x.NewContext(testNetworkID, testChainID, testAssetID, testFee, testFee)
}

func (*fakeNetwork) HRP() string { return testHRP }

func (*fakeNetwork) Asset(_ context.Context, asset string) (Asset, error) {
	if asset == "DJT" || asset == testAssetID.String() {
		return testAsset, nil
	}
	return Asset{}, errors.New("unknown asset")
}

// fund creates a UTXO of [amount] owned by [owner], produced by a stored
// transaction so that its owner can be looked up.
func (f *fakeNetwork) fund(owner *secp256k1fx.OutputOwners, amount uint64) {
	f.lock.Lock()
	defer f.lock.Unlock()
	memo := ids.GenerateTestID()
	tx := &txs.Tx{Unsigned: &txs.BaseTx{BaseTx: djtx.BaseTx{
		NetworkID:    testNetworkID,
		BlockchainID: testChainID,
		Outs: []*djtx.TransferableOutput{{
			Asset: djtx.Asset{ID: testAssetID},
			Out:   &secp256k1fx.TransferOutput{Amt: amount, OutputOwners: *owner},
		}},
		// Make every funding tx unique.
		Memo: memo[:],
	}}}
	if err := tx.SignSECP256K1Fx(x.Parser.Codec(), nil); err != nil {
		panic(err)
	}
	f.apply(tx)
}

func (f *fakeNetwork) apply(tx *txs.Tx) {
	if base, ok := tx.Unsigned.(*txs.BaseTx); ok {
		for _, in := range base.Ins {
			delete(f.utxos, in.InputID())
		}
	}
	for _, utxo := range tx.UTXOs() {
		f.utxos[utxo.InputID()] = utxo
	}
	f.txs[tx.ID()] = tx
	f.statuses[tx.ID()] = choices.Accepted
}

// register adds a version of a name record.
func (f *fakeNetwork) register(name string, target ids.ShortID, version uint64) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.records[name] = append(f.records[name], &platformvm.NameRecord{
		Name:       name,
		Status:     string(names.StatusActive),
		Target:     formatVAddress(testHRP, target),
		TargetID:   target,
		Expiry:     json.Uint64(4_000_000_000),
		Generation: 1,
		Version:    json.Uint64(version),
		RecordID:   ids.ID{'r', byte(len(name))},
	})
}

func (f *fakeNetwork) current(name string) *platformvm.NameRecord {
	versions := f.records[name]
	if len(versions) == 0 {
		return nil
	}
	return versions[len(versions)-1]
}

func (f *fakeNetwork) Resolve(_ context.Context, name string) (*platformvm.NameResolution, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.down {
		return nil, errors.New("connection refused")
	}
	reply := &platformvm.NameResolution{Name: name, Status: string(names.StatusNotFound)}
	if record := f.current(name); record != nil {
		clone := *record
		if f.expired[name] {
			clone.Status = string(names.StatusExpired)
		}
		reply.Record = &clone
		reply.Status = clone.Status
		reply.Resolvable = clone.Status == string(names.StatusActive)
	}
	return reply, nil
}

func (f *fakeNetwork) Reverse(_ context.Context, addr string) (*platformvm.ReverseResolution, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	reply := &platformvm.ReverseResolution{Address: addr}
	for name := range f.records {
		if record := f.current(name); record.Target == addr && !f.expired[name] {
			reply.Name = name
		}
	}
	return reply, nil
}

func (f *fakeNetwork) History(_ context.Context, name string) (*platformvm.NameHistoryReply, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	return &platformvm.NameHistoryReply{Name: name, Versions: f.records[name]}, nil
}

func (f *fakeNetwork) UTXOs(_ context.Context, addr ids.ShortID) ([]*djtx.UTXO, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.down {
		return nil, errors.New("connection refused")
	}
	var utxos []*djtx.UTXO
	for _, utxo := range f.utxos {
		out, ok := utxo.Out.(*secp256k1fx.TransferOutput)
		if !ok {
			continue
		}
		for _, owner := range out.Addrs {
			if owner == addr {
				utxos = append(utxos, utxo)
				break
			}
		}
	}
	return utxos, nil
}

func (f *fakeNetwork) IssueTx(_ context.Context, txBytes []byte) (ids.ID, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	tx, err := x.Parser.Parse(txBytes)
	if err != nil {
		return ids.Empty, &json2.Error{Message: err.Error()}
	}
	base := tx.Unsigned.(*txs.BaseTx)
	for _, in := range base.Ins {
		if _, ok := f.utxos[in.InputID()]; !ok {
			return ids.Empty, &json2.Error{Message: "missing utxo " + in.InputID().String()}
		}
	}
	if len(tx.Creds) != len(base.Ins) {
		return ids.Empty, &json2.Error{Message: "wrong number of credentials"}
	}
	f.issued++
	if f.hold {
		for _, in := range base.Ins {
			delete(f.utxos, in.InputID())
		}
		f.txs[tx.ID()] = tx
		f.statuses[tx.ID()] = choices.Processing
		return tx.ID(), nil
	}
	f.apply(tx)
	return tx.ID(), nil
}

func (f *fakeNetwork) TxStatus(_ context.Context, txID ids.ID) (choices.Status, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	status, ok := f.statuses[txID]
	if !ok {
		return choices.Unknown, nil
	}
	return status, nil
}

func (f *fakeNetwork) GetTx(_ context.Context, txID ids.ID) (*txs.Tx, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	tx, ok := f.txs[txID]
	if !ok {
		return nil, errors.New("not found")
	}
	return tx, nil
}
