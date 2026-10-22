package txservice

import (
	"errors"
	"math/big"
	"strings"

	"github.com/lasthyphen/dijetsnodesgo/utils/json"
)

var (
	errEmptyAmount      = errors.New("amount is empty")
	errMalformedAmount  = errors.New("amount must be a positive decimal number such as 10 or 0.5")
	errTooManyDecimals  = errors.New("amount has more decimal places than the asset supports")
	errZeroAmount       = errors.New("amount must be greater than zero")
	errAmountOutOfRange = errors.New("amount is too large")
	maxUint64           = new(big.Int).SetUint64(^uint64(0))
)

// ParseAmount converts a decimal string in whole units into base units of
// an asset with [denomination] decimal places. It accepts only plain
// decimals: no signs, exponents, separators or surrounding text.
func ParseAmount(s string, denomination int) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errEmptyAmount
	}
	whole, frac, hasPoint := strings.Cut(s, ".")
	if whole == "" || (hasPoint && frac == "") || !isDigits(whole) || !isDigits(frac) {
		return 0, errMalformedAmount
	}
	if len(frac) > denomination {
		return 0, errTooManyDecimals
	}
	digits := whole + frac + strings.Repeat("0", denomination-len(frac))
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return 0, errMalformedAmount
	}
	if n.Sign() == 0 {
		return 0, errZeroAmount
	}
	if n.Cmp(maxUint64) > 0 {
		return 0, errAmountOutOfRange
	}
	return n.Uint64(), nil
}

// FormatAmount renders base units as a decimal string without trailing
// zeros.
func FormatAmount(units uint64, denomination int) string {
	s := new(big.Int).SetUint64(units).String()
	if denomination == 0 {
		return s
	}
	if len(s) <= denomination {
		s = strings.Repeat("0", denomination-len(s)+1) + s
	}
	whole, frac := s[:len(s)-denomination], strings.TrimRight(s[len(s)-denomination:], "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

func newAmount(units uint64, denomination int) Amount {
	return Amount{
		Units:   json.Uint64(units),
		Display: FormatAmount(units, denomination),
	}
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
