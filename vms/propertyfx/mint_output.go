

package propertyfx

import (
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

type MintOutput struct {
	secp256k1fx.OutputOwners `serialize:"true"`
}
