

package signer

import (
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto/bls"
)

var _ Signer = (*Empty)(nil)

type Empty struct{}

func (*Empty) Verify() error {
	return nil
}

func (*Empty) Key() *bls.PublicKey {
	return nil
}
