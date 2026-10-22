package txservice

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// Output roles.
const (
	RolePayment = "payment"
	RoleChange  = "change"
	RoleUnknown = "unknown"
)

// AttributionNote is attached to every view so that no reader mistakes a
// name for a historical fact.
const AttributionNote = "Names are attributed from the naming registry's current state. " +
	"A name is shown only for an address with an ACTIVE registration that targets it. " +
	"V-chain transactions carry no timestamp, so a name shown here is not proof of who held the address when the transaction was accepted."

// TxView is the human-readable view of a V-chain transaction, derived only
// from the transaction, its inputs' producing transactions, and the naming
// registry.
type TxView struct {
	Envelope
	TxID   ids.ID `json:"txID"`
	Status string `json:"status"`
	Kind   string `json:"kind"`
	// From is the common owner of all inputs; nil if they differ or can't
	// be determined.
	From *OwnerView `json:"from,omitempty"`
	// To lists the owners of payment outputs.
	To      []*OwnerView `json:"to"`
	Inputs  []InputView  `json:"inputs"`
	Outputs []OutputView `json:"outputs"`
	// Fees maps asset symbol (or ID) to the amount burned.
	Fees        []AssetAmount `json:"fees"`
	Memo        string        `json:"memo,omitempty"`
	MemoClaim   *MemoClaim    `json:"memoClaim,omitempty"`
	Attribution string        `json:"attribution"`
}

// AddressView is an address and, when reliably known, its name.
type AddressView struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// OwnerView is an owner set with a human-readable label.
type OwnerView struct {
	Label     string        `json:"label"`
	Threshold int           `json:"threshold"`
	Locktime  json.Uint64   `json:"locktime"`
	Addresses []AddressView `json:"addresses"`
}

type AssetAmount struct {
	Asset  Asset  `json:"asset"`
	Amount Amount `json:"amount"`
}

type InputView struct {
	UTXO   string `json:"utxo"`
	Asset  Asset  `json:"asset"`
	Amount Amount `json:"amount"`
	// Owner is nil if the producing transaction could not be read.
	Owner *OwnerView `json:"owner,omitempty"`
}

type OutputView struct {
	Index  int       `json:"index"`
	Asset  Asset     `json:"asset"`
	Amount Amount    `json:"amount"`
	Owner  OwnerView `json:"owner"`
	Role   string    `json:"role"`
	// Evidence explains the role.
	Evidence string `json:"evidence"`
}

// MemoClaim is a name-payment memo checked against the registry.
type MemoClaim struct {
	Name       string `json:"name"`
	Generation uint64 `json:"generation"`
	Version    uint64 `json:"version"`
	// Verified is true only if that record version targeted a payment
	// output of this transaction.
	Verified bool   `json:"verified"`
	Detail   string `json:"detail"`
}

type describer struct {
	s      *Service
	ctx    context.Context
	hrp    string
	names  map[ids.ShortID]string
	assets map[ids.ID]Asset
	txs    map[ids.ID]*txs.Tx
}

func (s *Service) describe(ctx context.Context, txID ids.ID) (*TxView, error) {
	tx, err := s.network.GetTx(ctx, txID)
	if err != nil {
		return nil, apiError(CodeNotFound, "tx_not_found", "", "transaction %s could not be read: %s", txID, err)
	}
	status, err := s.network.TxStatus(ctx, txID)
	if err != nil {
		return nil, networkError(err)
	}
	d := &describer{
		s:      s,
		ctx:    ctx,
		hrp:    s.network.HRP(),
		names:  make(map[ids.ShortID]string),
		assets: make(map[ids.ID]Asset),
		txs:    map[ids.ID]*txs.Tx{txID: tx},
	}
	return d.view(tx, status)
}

func (d *describer) view(tx *txs.Tx, status choices.Status) (*TxView, error) {
	view := &TxView{
		Envelope:    envelope,
		TxID:        tx.ID(),
		Status:      status.String(),
		Kind:        strings.TrimPrefix(fmt.Sprintf("%T", tx.Unsigned), "*txs."),
		To:          []*OwnerView{},
		Inputs:      []InputView{},
		Outputs:     []OutputView{},
		Fees:        []AssetAmount{},
		Attribution: AttributionNote,
	}
	base, ok := baseOf(tx.Unsigned)
	if !ok {
		return view, nil
	}
	view.Memo = string(base.Memo)

	consumed := map[ids.ID]uint64{}
	produced := map[ids.ID]uint64{}

	// Inputs, with the owners of the outputs they spend.
	var (
		inputOwners []*secp256k1fx.OutputOwners
		ownersKnown = true
	)
	for _, in := range base.Ins {
		asset, err := d.asset(in.AssetID())
		if err != nil {
			return nil, err
		}
		consumed[in.AssetID()] += in.In.Amount()
		input := InputView{
			UTXO:   fmt.Sprintf("%s:%d", in.TxID, in.OutputIndex),
			Asset:  asset,
			Amount: newAmount(in.In.Amount(), asset.Denomination),
		}
		owner, ok := d.spentOwner(in)
		if ok {
			input.Owner = d.ownerView(owner)
			inputOwners = append(inputOwners, owner)
		} else {
			ownersKnown = false
		}
		view.Inputs = append(view.Inputs, input)
	}
	if ownersKnown && len(inputOwners) > 0 && allEqual(inputOwners) {
		view.From = d.ownerView(inputOwners[0])
	}

	// Outputs, classified by the evidence available.
	for i, out := range base.Outs {
		asset, err := d.asset(out.AssetID())
		if err != nil {
			return nil, err
		}
		output := OutputView{
			Index: i,
			Asset: asset,
			Role:  RoleUnknown,
		}
		transfer, ok := out.Out.(*secp256k1fx.TransferOutput)
		if !ok {
			output.Evidence = fmt.Sprintf("output type %T is not a transfer", out.Out)
			view.Outputs = append(view.Outputs, output)
			continue
		}
		produced[out.AssetID()] += transfer.Amt
		output.Amount = newAmount(transfer.Amt, asset.Denomination)
		output.Owner = *d.ownerView(&transfer.OutputOwners)
		switch {
		case !ownersKnown:
			output.Evidence = "the owners of the inputs could not be determined"
		case containsOwner(inputOwners, &transfer.OutputOwners):
			output.Role = RoleChange
			output.Evidence = "returns to the owner of the inputs"
		default:
			output.Role = RolePayment
			output.Evidence = "owned by someone other than the inputs' owner"
			view.To = append(view.To, d.ownerView(&transfer.OutputOwners))
		}
		view.Outputs = append(view.Outputs, output)
	}

	assetIDs := make([]ids.ID, 0, len(consumed))
	for assetID := range consumed {
		assetIDs = append(assetIDs, assetID)
	}
	sort.Slice(assetIDs, func(i, j int) bool { return assetIDs[i].String() < assetIDs[j].String() })
	for _, assetID := range assetIDs {
		if consumed[assetID] <= produced[assetID] {
			continue
		}
		asset := d.assets[assetID]
		view.Fees = append(view.Fees, AssetAmount{
			Asset:  asset,
			Amount: newAmount(consumed[assetID]-produced[assetID], asset.Denomination),
		})
	}

	if memo, ok := names.ParsePaymentMemo(base.Memo); ok {
		view.MemoClaim = d.checkMemo(memo, view.Outputs)
	}
	return view, nil
}

func baseOf(utx txs.UnsignedTx) (*txs.BaseTx, bool) {
	switch utx := utx.(type) {
	case *txs.BaseTx:
		return utx, true
	case *txs.ImportTx:
		return &utx.BaseTx, true
	case *txs.ExportTx:
		return &utx.BaseTx, true
	case *txs.OperationTx:
		return &utx.BaseTx, true
	case *txs.CreateAssetTx:
		return &utx.BaseTx, true
	default:
		return nil, false
	}
}

func (d *describer) asset(assetID ids.ID) (Asset, error) {
	if a, ok := d.assets[assetID]; ok {
		return a, nil
	}
	a, err := d.s.network.Asset(d.ctx, assetID.String())
	if err != nil {
		return Asset{}, networkError(err)
	}
	d.assets[assetID] = a
	return a, nil
}

// spentOwner returns the owner of the output an input spends, read from
// the transaction that produced it.
func (d *describer) spentOwner(in *djtx.TransferableInput) (*secp256k1fx.OutputOwners, bool) {
	parent, ok := d.txs[in.TxID]
	if !ok {
		var err error
		parent, err = d.s.network.GetTx(d.ctx, in.TxID)
		if err != nil {
			return nil, false
		}
		d.txs[in.TxID] = parent
	}
	for _, utxo := range parent.UTXOs() {
		if utxo.OutputIndex != in.OutputIndex {
			continue
		}
		if transfer, ok := utxo.Out.(*secp256k1fx.TransferOutput); ok {
			return &transfer.OutputOwners, true
		}
		return nil, false
	}
	return nil, false
}

func (d *describer) name(addr ids.ShortID) string {
	if name, ok := d.names[addr]; ok {
		return name
	}
	name := ""
	if reverse, err := d.s.network.Reverse(d.ctx, formatVAddress(d.hrp, addr)); err == nil {
		name = reverse.Name
	}
	d.names[addr] = name
	return name
}

func (d *describer) ownerView(owner *secp256k1fx.OutputOwners) *OwnerView {
	view := &OwnerView{
		Threshold: int(owner.Threshold),
		Locktime:  json.Uint64(owner.Locktime),
		Addresses: make([]AddressView, len(owner.Addrs)),
	}
	labels := make([]string, len(owner.Addrs))
	for i, addr := range owner.Addrs {
		address := formatVAddress(d.hrp, addr)
		name := d.name(addr)
		view.Addresses[i] = AddressView{Address: address, Name: name}
		labels[i] = name
		if name == "" {
			labels[i] = address
		}
	}
	switch {
	case len(labels) == 1 && owner.Threshold == 1:
		view.Label = labels[0]
	default:
		view.Label = fmt.Sprintf("%d-of-%d {%s}", owner.Threshold, len(labels), strings.Join(labels, ", "))
	}
	if owner.Locktime > 0 {
		view.Label += fmt.Sprintf(" (locked until %d)", owner.Locktime)
	}
	return view
}

// checkMemo verifies a name-payment memo: the named record version must
// have targeted one of the payment outputs.
func (d *describer) checkMemo(memo names.PaymentMemo, outputs []OutputView) *MemoClaim {
	claim := &MemoClaim{
		Name:       memo.Name,
		Generation: memo.Generation,
		Version:    memo.Version,
	}
	history, err := d.s.network.History(d.ctx, memo.Name)
	if err != nil {
		claim.Detail = "the registry could not be read"
		return claim
	}
	for _, record := range history.Versions {
		if uint64(record.Generation) != memo.Generation || uint64(record.Version) != memo.Version {
			continue
		}
		for _, out := range outputs {
			if out.Role == RolePayment && len(out.Owner.Addresses) == 1 && out.Owner.Addresses[0].Address == record.Target {
				claim.Verified = true
				claim.Detail = fmt.Sprintf("version %d of %q targeted the paid address", memo.Version, memo.Name)
				return claim
			}
		}
		claim.Detail = fmt.Sprintf("version %d of %q did not target any payment output", memo.Version, memo.Name)
		return claim
	}
	claim.Detail = fmt.Sprintf("the registry has no version %d (generation %d) of %q", memo.Version, memo.Generation, memo.Name)
	return claim
}

func allEqual(owners []*secp256k1fx.OutputOwners) bool {
	for _, owner := range owners[1:] {
		if !owner.Equals(owners[0]) {
			return false
		}
	}
	return true
}

func containsOwner(owners []*secp256k1fx.OutputOwners, owner *secp256k1fx.OutputOwners) bool {
	for _, o := range owners {
		if o.Equals(owner) {
			return true
		}
	}
	return false
}
