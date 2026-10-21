

package crypto

import (
	"errors"
)

var (
	errInvalidSigLen = errors.New("invalid signature length")
	errMutatedSig    = errors.New("signature was mutated from its original format")
)
