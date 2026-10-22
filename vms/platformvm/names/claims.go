package names

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/hashing"
)

// NameClaim identifies the registry slot of a name. Two pending
// transactions that touch the same claim conflict: the mempool admits only
// one of them, just as it admits only one spender of a UTXO.
func NameClaim(name string) ids.ID {
	return hashing.ComputeHash256Array([]byte("name:" + name))
}

// TargetClaim identifies the reverse-index slot of a target address.
func TargetClaim(target ids.ShortID) ids.ID {
	return hashing.ComputeHash256Array(append([]byte("target:"), target[:]...))
}

// MemoPrefix starts the memo a name-based payment carries. The memo is a
// claim made by the payer, not a proof: a reader may treat it as verified
// only after checking that the stated record version targeted the paid
// address.
const MemoPrefix = "djtname/1"

// PaymentMemo identifies the record version a payment was resolved against.
type PaymentMemo struct {
	Name       string
	Generation uint64
	Version    uint64
}

func (m PaymentMemo) String() string {
	return fmt.Sprintf("%s to=%s g=%d v=%d", MemoPrefix, m.Name, m.Generation, m.Version)
}

// ParsePaymentMemo parses a memo produced by PaymentMemo.String. It reports
// false for any memo that is not exactly in that format.
func ParsePaymentMemo(memo []byte) (PaymentMemo, bool) {
	fields := strings.Split(string(memo), " ")
	if len(fields) != 4 || fields[0] != MemoPrefix {
		return PaymentMemo{}, false
	}
	name, ok := strings.CutPrefix(fields[1], "to=")
	if !ok || Validate(name) != nil {
		return PaymentMemo{}, false
	}
	generation, ok := parseField(fields[2], "g=")
	if !ok {
		return PaymentMemo{}, false
	}
	version, ok := parseField(fields[3], "v=")
	if !ok {
		return PaymentMemo{}, false
	}
	m := PaymentMemo{Name: name, Generation: generation, Version: version}
	return m, m.String() == string(memo)
}

func parseField(field, prefix string) (uint64, bool) {
	s, ok := strings.CutPrefix(field, prefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && n > 0
}
