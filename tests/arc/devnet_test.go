//go:build arc_e2e

package arce2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/arc"
	"github.com/lasthyphen/dijetsnodesgo/arc/workflow"
	"github.com/lasthyphen/dijetsnodesgo/genesis"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"
)

// TestDevNetwork starts the 5-node network with two healthy Arcs
// (alpha, beta) and one failed one (gamma), then keeps it running for
// explorer development. Skipped unless ARC_DEVNET=1. Run with:
//
//	ARC_DEVNET=1 ARC_DEVNET_HOLD=2h \
//	  go test -tags arc_e2e -timeout 0 -v -run TestDevNetwork ./tests/arc/
//
// Stop it with Ctrl-C; all nodes are stopped on exit.
func TestDevNetwork(t *testing.T) {
	if os.Getenv("ARC_DEVNET") != "1" {
		t.Skip("set ARC_DEVNET=1 to start the development network")
	}
	hold := 30 * time.Minute
	if v := os.Getenv("ARC_DEVNET_HOLD"); v != "" {
		d, err := time.ParseDuration(v)
		require.NoError(t, err, "ARC_DEVNET_HOLD")
		hold = d
	}

	net := newNetwork(t)
	kc := secp256k1fx.NewKeychain(genesis.EWOQKey)
	owner := &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{genesis.EWOQKey.PublicKey().Address()}}

	net.startAll()
	n1 := net.node(1)
	pClient := platformvm.NewClient(n1.URI())
	mc := arc.NewClient(n1.URI())
	wallet, err := primary.NewWalletFromURI(ctxWithTimeout(t, time.Minute), n1.URI(), kc)
	require.NoError(t, err)
	pWallet := wallet.P()

	alpha := &testArc{name: "alpha", members: []int{1, 2, 3}, genesis: buildGenesis(t, "0x616c706861")}
	beta := &testArc{name: "beta", members: []int{3, 4, 5}, genesis: buildGenesis(t, "0x62657461")}
	for _, c := range []*testArc{alpha, beta} {
		c.subnetID, err = pWallet.IssueCreateSubnetTx(owner, common.WithContext(ctxWithTimeout(t, txTimeout)))
		require.NoError(t, err)
	}
	// node3 and node4 track both subnets, so either can observe every chain.
	net.node(1).whitelist = []ids.ID{alpha.subnetID}
	net.node(2).whitelist = []ids.ID{alpha.subnetID}
	net.node(3).whitelist = []ids.ID{alpha.subnetID, beta.subnetID}
	net.node(4).whitelist = []ids.ID{alpha.subnetID, beta.subnetID}
	net.node(5).whitelist = []ids.ID{beta.subnetID}
	net.restartAll()

	for _, c := range []*testArc{alpha, beta} {
		committee := make([]workflow.Member, 0, len(c.members))
		for _, idx := range c.members {
			committee = append(committee, workflow.Member{NodeID: net.node(idx).nodeID, Weight: 20})
		}
		start := time.Now().Add(20 * time.Second)
		res, err := workflow.Create(ctxWithTimeout(t, 5*time.Minute), pWallet, pClient, mc, workflow.Request{
			Name:            c.name,
			Genesis:         c.genesis,
			SubnetID:        c.subnetID,
			Committee:       committee,
			ValidationStart: start,
			ValidationEnd:   start.Add(hold + 25*time.Hour),
		})
		require.NoError(t, err)
		c.chainID = res.ChainID
	}
	for _, c := range []*testArc{alpha, beta} {
		for _, idx := range c.members {
			awaitLifecycle(t, net.node(idx), c.chainID, arc.Active)
		}
	}
	gammaID, err := pWallet.IssueCreateChainTx(beta.subnetID, []byte("not an avm genesis"), constants.AVMID, workflow.FxIDs, "gamma",
		common.WithContext(ctxWithTimeout(t, txTimeout)))
	require.NoError(t, err)
	awaitLifecycle(t, net.node(5), gammaID, arc.Failed)

	alphaWallet, alphaAsset := chainWallet(t, net.node(1).URI(), alpha.chainID, kc)
	betaWallet, betaAsset := chainWallet(t, net.node(5).URI(), beta.chainID, kc)

	uris := make([]string, 0, numNodes)
	for _, nd := range net.nodes {
		uris = append(uris, nd.URI())
	}
	t.Logf("dev network ready for %s", hold)
	t.Logf("  nodes:  %s", strings.Join(uris, " "))
	t.Logf("  alpha:  %s (committee node1-3)", alpha.chainID)
	t.Logf("  beta:   %s (committee node3-5)", beta.chainID)
	t.Logf("  gamma:  %s (failed: invalid genesis)", gammaID)
	t.Logf("  explorer: ?registry=%s&observers=%s,%s", n1.URI(), net.node(3).URI(), net.node(4).URI())

	// Keep a little activity going so observed statistics change.
	recipient := ids.GenerateTestShortID()
	deadline := time.Now().Add(hold)
	for time.Now().Before(deadline) {
		if _, err := send(alphaWallet, alphaAsset, recipient, 1, common.WithContext(ctxWithTimeout(t, txTimeout))); err != nil {
			t.Logf("alpha tx failed: %v", err)
		}
		if _, err := send(betaWallet, betaAsset, recipient, 1, common.WithContext(ctxWithTimeout(t, txTimeout))); err != nil {
			t.Logf("beta tx failed: %v", err)
		}
		time.Sleep(20 * time.Second)
	}
}
