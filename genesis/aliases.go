// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package genesis

import (
	"path"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/nftfx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/genesis"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/propertyfx"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// Aliases returns the default aliases based on the network ID
func Aliases(genesisBytes []byte) (map[string][]string, map[ids.ID][]string, error) {
	apiAliases := map[string][]string{
		path.Join(constants.ChainAliasPrefix, constants.PlatformChainID.String()): {
			"M",
			"platform",
			path.Join(constants.ChainAliasPrefix, "M"),
			path.Join(constants.ChainAliasPrefix, "platform"),
		},
	}
	chainAliases := map[ids.ID][]string{
		constants.PlatformChainID: {"M", "platform"},
	}

	genesis, err := genesis.Parse(genesisBytes) // TODO let's not re-create genesis to do aliasing
	if err != nil {
		return nil, nil, err
	}
	for _, chain := range genesis.Chains {
		uChain := chain.Unsigned.(*txs.CreateChainTx)
		chainID := chain.ID()
		endpoint := path.Join(constants.ChainAliasPrefix, chainID.String())
		switch uChain.VMID {
		case constants.AVMID:
			apiAliases[endpoint] = []string{
				"V",
				"avm",
				path.Join(constants.ChainAliasPrefix, "V"),
				path.Join(constants.ChainAliasPrefix, "avm"),
			}
			chainAliases[chainID] = GetXChainAliases()
		case constants.EVMID:
			apiAliases[endpoint] = []string{
				"U",
				"evm",
				path.Join(constants.ChainAliasPrefix, "U"),
				path.Join(constants.ChainAliasPrefix, "evm"),
			}
			chainAliases[chainID] = GetCChainAliases()
		}
	}
	return apiAliases, chainAliases, nil
}

func GetCChainAliases() []string {
	return []string{"U", "evm"}
}

func GetXChainAliases() []string {
	return []string{"V", "avm"}
}

func GetVMAliases() map[ids.ID][]string {
	return map[ids.ID][]string{
		constants.PlatformVMID: {"platform"},
		constants.AVMID:        {"avm"},
		constants.EVMID:        {"evm"},
		secp256k1fx.ID:         {"secp256k1fx"},
		nftfx.ID:               {"nftfx"},
		propertyfx.ID:          {"propertyfx"},
	}
}
