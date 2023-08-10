// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package units

// Denominations of value
const (
	NanoDjtx  uint64 = 1
	MicroDjtx uint64 = 1000 * NanoDjtx
	Schmeckle uint64 = 49*MicroDjtx + 463*NanoDjtx
	MilliDjtx uint64 = 1000 * MicroDjtx
	Djtx      uint64 = 1000 * MilliDjtx
	KiloDjtx  uint64 = 1000 * Djtx
	MegaDjtx  uint64 = 1000 * KiloDjtx
)
