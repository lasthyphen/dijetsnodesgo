package names

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
)

// Params are the consensus-critical parameters of the registry. They are
// fixed per network rather than read from node flags, because every
// validator must apply the same values.
type Params struct {
	// MinTerm and MaxTerm bound the distance between the block timestamp and
	// the expiry a registration or renewal requests.
	MinTerm time.Duration `json:"minTerm"`
	MaxTerm time.Duration `json:"maxTerm"`
	// GracePeriod is how long an expired or revoked name stays unavailable
	// to other registrants.
	GracePeriod time.Duration `json:"gracePeriod"`
	// RegisterFee is burned by a RegisterNameTx, in nDJT. An UpdateNameTx
	// pays the ordinary TxFee.
	RegisterFee uint64 `json:"registerFee"`
}

var (
	// PublicParams apply to mainnet and Lothar once naming is scheduled
	// there.
	PublicParams = Params{
		MinTerm:     30 * 24 * time.Hour,
		MaxTerm:     5 * 365 * 24 * time.Hour,
		GracePeriod: 30 * 24 * time.Hour,
		RegisterFee: units.Djtx,
	}

	// DevelopmentParams apply to local and custom networks. The short terms
	// let tests exercise expiry, grace and release.
	DevelopmentParams = Params{
		MinTerm:     time.Minute,
		MaxTerm:     5 * 365 * 24 * time.Hour,
		GracePeriod: time.Minute,
		RegisterFee: 10 * units.MilliDjtx,
	}
)

// GetParams returns the registry parameters of [networkID].
func GetParams(networkID uint32) Params {
	switch networkID {
	case constants.MainnetID, constants.LotharID:
		return PublicParams
	default:
		return DevelopmentParams
	}
}
