//go:build names_e2e

package namese2e

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/genesis"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/tests/localnet"
	"github.com/lasthyphen/dijetsnodesgo/txservice"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
)

const devnetTxServicePort = 19800

func randomToken(t *testing.T) string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

// TestDevNetwork runs a local network with the Bob demo's names, a funded
// allowance and the transaction service, then holds it so that the web demo
// can be used against it. It is skipped unless NAMES_DEVNET=1:
//
//	NAMES_DEVNET=1 go test -tags names_e2e -timeout 0 -v -run TestDevNetwork ./tests/names/
//
// NAMES_DEVNET_HOLD sets how long to hold the network (default 2h). The
// test writes bob-demo.env into the network directory.
func TestDevNetwork(t *testing.T) {
	if os.Getenv("NAMES_DEVNET") != "1" {
		t.Skip("set NAMES_DEVNET=1 to run the development network")
	}
	hold := 2 * time.Hour
	if v := os.Getenv("NAMES_DEVNET_HOLD"); v != "" {
		var err error
		hold, err = time.ParseDuration(v)
		require.NoError(t, err)
	}

	net0 := localnet.New(t, localnet.Options{Name: "names-devnet", BasePort: 19750})
	net0.StartAll()
	n1 := net0.Node(1)

	keyDir := filepath.Join(net0.Dir, "keys")
	agent, err := txservice.LoadOrCreateKey(filepath.Join(keyDir, "bob.key"))
	require.NoError(t, err)
	keys := map[string]*crypto.PrivateKeySECP256K1R{"bob": agent}
	for _, name := range []string{"alice", "demo-user", "dijets-cafe", "civic-demo"} {
		keys[name], err = txservice.LoadOrCreateKey(filepath.Join(keyDir, name+".key"))
		require.NoError(t, err)
	}

	kc := secp256k1fx.NewKeychain(genesis.EWOQKey)
	for _, key := range keys {
		kc.Add(key)
	}
	wallet, err := primary.NewWalletFromURI(ctxWithTimeout(t, time.Minute), n1.URI(), kc)
	require.NoError(t, err)
	h := &harness{
		t:       t,
		net:     net0,
		pClient: platformvm.NewClient(n1.URI()),
		names:   platformvm.NewNamesClient(n1.URI()),
		pWallet: wallet.P(),
	}
	h.params, err = h.names.Params(ctxWithTimeout(t, 30*time.Second))
	require.NoError(t, err)
	for name, key := range keys {
		_, err := h.register(name, key, time.Now().Add(30*24*time.Hour))
		require.NoError(t, err, name)
		t.Logf("registered %-12s -> %s", name, vAddr(t, key))
	}

	ewoqWallet, err := primary.NewWalletFromURI(ctxWithTimeout(t, time.Minute), n1.URI(), secp256k1fx.NewKeychain(genesis.EWOQKey))
	require.NoError(t, err)
	asset, err := avm.NewClient(n1.URI(), "V").GetAssetDescription(ctxWithTimeout(t, 30*time.Second), "DJT")
	require.NoError(t, err)
	allowance := &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{keys["demo-user"].PublicKey().Address(), agent.PublicKey().Address()}}
	allowance.Sort()
	_, err = ewoqWallet.X().IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: asset.AssetID},
		Out:   &secp256k1fx.TransferOutput{Amt: 100 * units.Djtx, OutputOwners: *allowance},
	}})
	require.NoError(t, err)

	agentToken, approverToken := randomToken(t), randomToken(t)
	config := &txservice.Config{
		NodeURI: n1.URI(),
		Listen:  "127.0.0.1:" + strconv.Itoa(devnetTxServicePort),
		DataDir: filepath.Join(net0.Dir, "txservice"),
		Clients: []txservice.ClientConfig{
			{Name: "bob-demo-agent", TokenSHA256: txservice.HashToken(agentToken), Role: txservice.RoleAgent, Delegations: []string{"demo"}},
			{Name: "bob-demo-approver", TokenSHA256: txservice.HashToken(approverToken), Role: txservice.RoleApprover, Delegations: []string{"demo"}},
		},
		Delegations: []txservice.DelegationConfig{{
			ID:               "demo",
			Principal:        "demo-user",
			PrincipalAddress: vAddr(t, keys["demo-user"]),
			AgentName:        "bob",
			AgentKeyFile:     filepath.Join(keyDir, "bob.key"),
			Assets:           []string{"DJT"},
			PerPaymentMax:    "25",
			DailyMax:         "60",
			AutoApproveMax:   "0",
		}},
	}
	network, err := txservice.NewNodeNetwork(ctxWithTimeout(t, 30*time.Second), n1.URI())
	require.NoError(t, err)
	svc, err := txservice.NewService(ctxWithTimeout(t, 30*time.Second), config, network, log.New(io.Discard, "", 0), time.Now)
	require.NoError(t, err)
	defer svc.Close()
	handler, err := txservice.NewHandler(svc)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", config.Listen)
	require.NoError(t, err)
	server := txservice.NewServer(config, handler)
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	envPath := filepath.Join(net0.Dir, "bob-demo.env")
	env := fmt.Sprintf("TXSERVICE_URL=http://%s\nTXSERVICE_AGENT_TOKEN=%s\nTXSERVICE_APPROVER_TOKEN=%s\n", config.Listen, agentToken, approverToken)
	require.NoError(t, os.WriteFile(envPath, []byte(env), 0o600))

	t.Logf("node1 API:            %s", n1.URI())
	t.Logf("transaction service:  http://%s%s", config.Listen, txservice.Endpoint)
	t.Logf("bob-demo environment: %s", envPath)
	t.Logf("start the demo:       (cd bob-demo && env $(cat %s | xargs) npm start)", envPath)
	t.Logf("holding for %s; interrupt to stop", hold)

	deadline := time.After(hold)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return
		case <-ticker.C:
			if resp, err := http.Get("http://" + config.Listen + "/healthz"); err == nil {
				_ = resp.Body.Close()
			}
		}
	}
}
