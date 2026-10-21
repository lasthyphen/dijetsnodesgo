

package state

import (
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
)

type UTXOGetter interface {
	GetUTXO(utxoID ids.ID) (*djtx.UTXO, error)
}

type UTXOAdder interface {
	AddUTXO(utxo *djtx.UTXO)
}

type UTXODeleter interface {
	DeleteUTXO(utxoID ids.ID)
}
