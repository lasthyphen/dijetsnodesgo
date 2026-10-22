package txservice

import (
	"bytes"
	"fmt"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/utils/set"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

type avmTx = txs.Tx

func jsonUint32(v uint32) json.Uint32 {
	return json.Uint32(v)
}

func setOf(addrs ...ids.ShortID) set.Set[ids.ShortID] {
	s := set.NewSet[ids.ShortID](len(addrs))
	s.Add(addrs...)
	return s
}

// checkPaymentTx verifies a signed transaction does exactly what the
// approved payment says before it is issued: one output of the amount to
// the recipient, every other output returning change to the allowance, the
// approved memo, and a burn equal to the fee.
func checkPaymentTx(tx *avmTx, d *delegation, p *Payment, target ids.ShortID, feeAsset ids.ID) error {
	utx, ok := tx.Unsigned.(*txs.BaseTx)
	if !ok {
		return fmt.Errorf("unexpected tx type %T", tx.Unsigned)
	}
	if !bytes.Equal(utx.Memo, []byte(p.Memo)) {
		return fmt.Errorf("memo %q differs from the approved memo", utx.Memo)
	}
	var (
		consumed, produced uint64
		payments           int
	)
	for _, in := range utx.Ins {
		if in.AssetID() != p.Asset.ID {
			return fmt.Errorf("input of unexpected asset %s", in.AssetID())
		}
		consumed += in.In.Amount()
	}
	for _, out := range utx.Outs {
		transfer, ok := out.Out.(*secp256k1fx.TransferOutput)
		if !ok || out.AssetID() != p.Asset.ID {
			return fmt.Errorf("unexpected output %T of asset %s", out.Out, out.AssetID())
		}
		produced += transfer.Amt
		recipient := secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{target}}
		switch {
		case transfer.OutputOwners.Equals(&recipient) && transfer.Amt == uint64(p.Amount.Units):
			payments++
		case transfer.OutputOwners.Equals(d.owner):
		default:
			return fmt.Errorf("output of %d to an unexpected owner", transfer.Amt)
		}
	}
	if payments != 1 {
		return fmt.Errorf("expected one payment output, found %d", payments)
	}
	if consumed < produced {
		return fmt.Errorf("produces %d but consumes only %d", produced, consumed)
	}
	if burned := consumed - produced; p.Asset.ID == feeAsset && burned != uint64(p.Fee.Units) {
		return fmt.Errorf("burns %d instead of the fee %d", burned, uint64(p.Fee.Units))
	}
	return nil
}
