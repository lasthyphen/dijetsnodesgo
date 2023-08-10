// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package p

import (
	"context"
	"errors"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"

	"github.com/onsi/gomega"

	"github.com/lasthyphen/dijetsnodesgo/api/info"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/tests"
	"github.com/lasthyphen/dijetsnodesgo/tests/e2e"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/status"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/validator"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"
)

// PChainWorkflow is an integration test for normal P-Chain operations
// - Issues an Add Validator and an Add Delegator using the funding address
// - Exports DJTX from the P-Chain funding address to the X-Chain created address
// - Exports DJTX from the X-Chain created address to the P-Chain created address
// - Checks the expected value of the funding address

var _ = e2e.DescribePChain("[Workflow]", func() {
	ginkgo.It("P-chain main operations",
		// use this for filtering tests by labels
		// ref. https://onsi.github.io/ginkgo/#spec-labels
		ginkgo.Label(
			"require-network-runner",
			"xp",
			"workflow",
		),
		ginkgo.FlakeAttempts(2),
		func() {
			rpcEps := e2e.Env.GetURIs()
			gomega.Expect(rpcEps).ShouldNot(gomega.BeEmpty())
			nodeURI := rpcEps[0]

			tests.Outf("{{blue}} setting up keys {{/}}\n")
			_, testKeyAddrs, keyChain := e2e.Env.GetTestKeys()

			tests.Outf("{{blue}} setting up wallet {{/}}\n")
			ctx, cancel := context.WithTimeout(context.Background(), e2e.DefaultWalletCreationTimeout)
			baseWallet, err := primary.NewWalletFromURI(ctx, nodeURI, keyChain)
			cancel()
			gomega.Expect(err).Should(gomega.BeNil())

			pWallet := baseWallet.P()
			djtxAssetID := baseWallet.P().DJTXAssetID()
			xWallet := baseWallet.X()
			pChainClient := platformvm.NewClient(nodeURI)
			xChainClient := avm.NewClient(nodeURI, xWallet.BlockchainID().String())

			tests.Outf("{{blue}} fetching minimal stake amounts {{/}}\n")
			ctx, cancel = context.WithTimeout(context.Background(), e2e.DefaultWalletCreationTimeout)
			minValStake, minDelStake, err := pChainClient.GetMinStake(ctx, constants.PlatformChainID)
			cancel()
			gomega.Expect(err).Should(gomega.BeNil())
			tests.Outf("{{green}} minimal validator stake: %d {{/}}\n", minValStake)
			tests.Outf("{{green}} minimal delegator stake: %d {{/}}\n", minDelStake)

			tests.Outf("{{blue}} fetching tx fee {{/}}\n")
			infoClient := info.NewClient(nodeURI)
			ctx, cancel = context.WithTimeout(context.Background(), e2e.DefaultWalletCreationTimeout)
			fees, err := infoClient.GetTxFee(ctx)
			cancel()
			gomega.Expect(err).Should(gomega.BeNil())
			txFees := uint64(fees.TxFee)
			tests.Outf("{{green}} txFee: %d {{/}}\n", txFees)

			// amount to transfer from P to X chain
			toTransfer := 1 * units.Djtx

			pShortAddr := testKeyAddrs[0]
			xTargetAddr := testKeyAddrs[1]
			ginkgo.By("check selected keys have sufficient funds", func() {
				pBalances, err := pWallet.Builder().GetBalance()
				pBalance := pBalances[djtxAssetID]
				minBalance := minValStake + txFees + minDelStake + txFees + toTransfer + txFees
				gomega.Expect(pBalance, err).To(gomega.BeNumerically(">=", minBalance))
			})
			// create validator data
			validatorStartTimeDiff := 30 * time.Second
			vdrStartTime := time.Now().Add(validatorStartTimeDiff)

			vdr := &validator.Validator{
				NodeID: ids.GenerateTestNodeID(),
				Start:  uint64(vdrStartTime.Unix()),
				End:    uint64(vdrStartTime.Add(72 * time.Hour).Unix()),
				Wght:   minValStake,
			}
			rewardOwner := &secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{pShortAddr},
			}
			shares := uint32(20000) // TODO: retrieve programmatically

			ginkgo.By("issue add validator tx", func() {
				ctx, cancel := context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				addValidatorTxID, err := pWallet.IssueAddValidatorTx(
					vdr,
					rewardOwner,
					shares,
					common.WithContext(ctx),
				)
				cancel()
				gomega.Expect(err).Should(gomega.BeNil())

				ctx, cancel = context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				txStatus, err := pChainClient.GetTxStatus(ctx, addValidatorTxID)
				cancel()
				gomega.Expect(txStatus.Status, err).To(gomega.Equal(status.Committed))
			})

			ginkgo.By("issue add delegator tx", func() {
				ctx, cancel := context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				addDelegatorTxID, err := pWallet.IssueAddDelegatorTx(
					vdr,
					rewardOwner,
					common.WithContext(ctx),
				)
				cancel()
				gomega.Expect(err).Should(gomega.BeNil())

				ctx, cancel = context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				txStatus, err := pChainClient.GetTxStatus(ctx, addDelegatorTxID)
				cancel()
				gomega.Expect(txStatus.Status, err).To(gomega.Equal(status.Committed))
			})

			// retrieve initial balances
			pBalances, err := pWallet.Builder().GetBalance()
			gomega.Expect(err).Should(gomega.BeNil())
			pStartBalance := pBalances[djtxAssetID]
			tests.Outf("{{blue}} P-chain balance before P->X export: %d {{/}}\n", pStartBalance)

			xBalances, err := xWallet.Builder().GetFTBalance()
			gomega.Expect(err).Should(gomega.BeNil())
			xStartBalance := xBalances[djtxAssetID]
			tests.Outf("{{blue}} X-chain balance before P->X export: %d {{/}}\n", xStartBalance)

			outputOwner := secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					xTargetAddr,
				},
			}
			output := &secp256k1fx.TransferOutput{
				Amt:          toTransfer,
				OutputOwners: outputOwner,
			}

			ginkgo.By("export djtx from P to X chain", func() {
				ctx, cancel := context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				exportTxID, err := pWallet.IssueExportTx(
					xWallet.BlockchainID(),
					[]*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: djtxAssetID,
							},
							Out: output,
						},
					},
					common.WithContext(ctx),
				)
				cancel()
				gomega.Expect(err).Should(gomega.BeNil())

				ctx, cancel = context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				txStatus, err := pChainClient.GetTxStatus(ctx, exportTxID)
				cancel()
				gomega.Expect(txStatus.Status, err).To(gomega.Equal(status.Committed))
			})

			// check balances post export
			pBalances, err = pWallet.Builder().GetBalance()
			gomega.Expect(err).Should(gomega.BeNil())
			pPreImportBalance := pBalances[djtxAssetID]
			tests.Outf("{{blue}} P-chain balance after P->X export: %d {{/}}\n", pPreImportBalance)

			xBalances, err = xWallet.Builder().GetFTBalance()
			gomega.Expect(err).Should(gomega.BeNil())
			xPreImportBalance := xBalances[djtxAssetID]
			tests.Outf("{{blue}} X-chain balance after P->X export: %d {{/}}\n", xPreImportBalance)

			gomega.Expect(xPreImportBalance).To(gomega.Equal(xStartBalance)) // import not performed yet
			gomega.Expect(pPreImportBalance).To(gomega.Equal(pStartBalance - toTransfer - txFees))

			ginkgo.By("import djtx from P into X chain", func() {
				ctx, cancel := context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				importTxID, err := xWallet.IssueImportTx(
					constants.PlatformChainID,
					&outputOwner,
					common.WithContext(ctx),
				)
				cancel()
				gomega.Expect(err).Should(gomega.BeNil(), fmt.Errorf("error timeout: %v", errors.Is(err, context.DeadlineExceeded)))

				ctx, cancel = context.WithTimeout(context.Background(), e2e.DefaultConfirmTxTimeout)
				txStatus, err := xChainClient.GetTxStatus(ctx, importTxID)
				cancel()
				gomega.Expect(txStatus, err).To(gomega.Equal(choices.Accepted))
			})

			// check balances post import
			pBalances, err = pWallet.Builder().GetBalance()
			gomega.Expect(err).Should(gomega.BeNil())
			pFinalBalance := pBalances[djtxAssetID]
			tests.Outf("{{blue}} P-chain balance after P->X import: %d {{/}}\n", pFinalBalance)

			xBalances, err = xWallet.Builder().GetFTBalance()
			gomega.Expect(err).Should(gomega.BeNil())
			xFinalBalance := xBalances[djtxAssetID]
			tests.Outf("{{blue}} X-chain balance after P->X import: %d {{/}}\n", xFinalBalance)

			gomega.Expect(xFinalBalance).To(gomega.Equal(xPreImportBalance + toTransfer - txFees)) // import not performed yet
			gomega.Expect(pFinalBalance).To(gomega.Equal(pPreImportBalance))
		})
})
