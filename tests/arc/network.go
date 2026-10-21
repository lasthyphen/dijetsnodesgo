//go:build arc_e2e

// Package arce2e runs a real 5-node staking network locally and
// exercises Arcs end to end. Run with:
//
//	./scripts/build_dijets.sh
//	go test -tags arc_e2e -timeout 60m -v ./tests/arc/
package arce2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/api/info"
	"github.com/lasthyphen/dijetsnodesgo/ids"
)

const (
	networkID     = 1337
	numNodes      = 5
	basePort      = 19650
	bootstrapWait = 5 * time.Minute
)

// Node IDs of staking/local/staker{1..5}.crt, in order.
var stakerNodeIDs = []string{
	"NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg",
	"NodeID-MFrZFVCXPv5iCn6M9K6XduxGTYp891xXZ",
	"NodeID-NFBbbJ4qCmNaCzeW7sxErhvWqvEQMnYcN",
	"NodeID-GWPcbFJZFfZreETSoWjPimr846mXEKCtu",
	"NodeID-P7oB2McjBGgW2NXXWVYjV8JEDFoW9xDE5",
}

type node struct {
	idx         int
	nodeID      ids.NodeID
	httpPort    int
	stakingPort int
	dataDir     string
	cmd         *exec.Cmd
	exited      chan struct{}
	whitelist   []ids.ID
}

func (n *node) URI() string { return fmt.Sprintf("http://127.0.0.1:%d", n.httpPort) }

func (n *node) running() bool {
	if n.cmd == nil {
		return false
	}
	select {
	case <-n.exited:
		return false
	default:
		return true
	}
}

type network struct {
	t           *testing.T
	repoRoot    string
	binary      string
	dir         string
	genesisPath string
	pluginDir   string
	nodes       []*node
}

func repoRoot(t *testing.T) string {
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func newNetwork(t *testing.T) *network {
	root := repoRoot(t)
	binary := os.Getenv("ARC_NODE_BINARY")
	if binary == "" {
		binary = filepath.Join(root, "build", "dijetsnodesgo")
	}
	_, err := os.Stat(binary)
	require.NoError(t, err, "build the node first: ./scripts/build_dijets.sh")

	dir, err := os.MkdirTemp("", "arc-e2e-")
	require.NoError(t, err)
	t.Logf("network directory (kept for inspection): %s", dir)

	n := &network{
		t:         t,
		repoRoot:  root,
		binary:    binary,
		dir:       dir,
		pluginDir: filepath.Join(dir, "plugins"),
	}
	require.NoError(t, os.MkdirAll(n.pluginDir, 0o750))
	n.writeGenesis()

	for i := 0; i < numNodes; i++ {
		nodeID, err := ids.NodeIDFromString(stakerNodeIDs[i])
		require.NoError(t, err)
		n.nodes = append(n.nodes, &node{
			idx:         i + 1,
			nodeID:      nodeID,
			httpPort:    basePort + 2*i,
			stakingPort: basePort + 2*i + 1,
			dataDir:     filepath.Join(dir, fmt.Sprintf("node%d", i+1)),
		})
	}
	t.Cleanup(n.stopAll)
	return n
}

// writeGenesis derives a custom-network genesis from the local genesis with a
// recent start time; the shipped local genesis' staking periods have expired.
func (n *network) writeGenesis() {
	raw, err := os.ReadFile(filepath.Join(n.repoRoot, "genesis", "genesis_local.json"))
	require.NoError(n.t, err)
	var g map[string]interface{}
	require.NoError(n.t, json.Unmarshal(raw, &g))
	g["networkID"] = networkID
	g["startTime"] = time.Now().Add(-time.Minute).Unix()
	g["message"] = "arc e2e"
	out, err := json.MarshalIndent(g, "", "  ")
	require.NoError(n.t, err)
	n.genesisPath = filepath.Join(n.dir, "genesis.json")
	require.NoError(n.t, os.WriteFile(n.genesisPath, out, 0o600))
}

func (n *network) node(i int) *node { return n.nodes[i-1] }

func (n *network) start(nd *node) {
	args := []string{
		fmt.Sprintf("--network-id=%d", networkID),
		"--genesis=" + n.genesisPath,
		"--data-dir=" + nd.dataDir,
		// Plugins are loaded from <build-dir>/plugins; use an empty one. The
		// U-chain EVM is compiled into the node.
		"--build-dir=" + n.dir,
		"--http-host=127.0.0.1",
		fmt.Sprintf("--http-port=%d", nd.httpPort),
		fmt.Sprintf("--staking-port=%d", nd.stakingPort),
		"--public-ip=127.0.0.1",
		fmt.Sprintf("--staking-tls-cert-file=%s", filepath.Join(n.repoRoot, "staking", "local", fmt.Sprintf("staker%d.crt", nd.idx))),
		fmt.Sprintf("--staking-tls-key-file=%s", filepath.Join(n.repoRoot, "staking", "local", fmt.Sprintf("staker%d.key", nd.idx))),
		"--log-level=info",
		"--log-display-level=warn",
		"--health-check-frequency=2s",
	}
	if nd.idx != 1 {
		first := n.nodes[0]
		args = append(args,
			fmt.Sprintf("--bootstrap-ips=127.0.0.1:%d", first.stakingPort),
			"--bootstrap-ids="+first.nodeID.String(),
		)
	}
	if len(nd.whitelist) > 0 {
		subnets := make([]string, len(nd.whitelist))
		for i, s := range nd.whitelist {
			subnets[i] = s.String()
		}
		args = append(args, "--whitelisted-subnets="+strings.Join(subnets, ","))
	}

	require.NoError(n.t, os.MkdirAll(nd.dataDir, 0o750))
	logFile, err := os.OpenFile(filepath.Join(nd.dataDir, "stdout.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(n.t, err)
	cmd := exec.Command(n.binary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(n.t, cmd.Start())
	nd.cmd = cmd
	nd.exited = make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		close(nd.exited)
	}()
	n.t.Logf("started node%d (%s) whitelist=%v", nd.idx, nd.nodeID, nd.whitelist)
}

func (n *network) stop(nd *node) {
	if !nd.running() {
		return
	}
	_ = nd.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-nd.exited:
	case <-time.After(60 * time.Second):
		_ = nd.cmd.Process.Kill()
		<-nd.exited
	}
	n.t.Logf("stopped node%d", nd.idx)
}

func (n *network) stopAll() {
	for _, nd := range n.nodes {
		n.stop(nd)
	}
}

func (n *network) startAll() {
	for _, nd := range n.nodes {
		n.start(nd)
	}
	for _, nd := range n.nodes {
		n.awaitBootstrapped(nd, "M", "V", "U")
	}
}

func (n *network) restartAll() {
	n.stopAll()
	n.startAll()
}

func (n *network) awaitBootstrapped(nd *node, chains ...string) {
	client := info.NewClient(nd.URI())
	eventually(n.t, bootstrapWait, time.Second, func() (bool, string) {
		if !nd.running() {
			n.t.Fatalf("node%d exited; see %s", nd.idx, filepath.Join(nd.dataDir, "stdout.log"))
		}
		for _, chain := range chains {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			ok, err := client.IsBootstrapped(ctx, chain)
			cancel()
			if err != nil || !ok {
				return false, fmt.Sprintf("node%d chain %s not bootstrapped (err=%v)", nd.idx, chain, err)
			}
		}
		return true, ""
	})
}

func eventually(t *testing.T, timeout, interval time.Duration, cond func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		ok, msg := cond()
		if ok {
			return
		}
		last = msg
		if time.Now().After(deadline) {
			t.Fatalf("condition not met after %s: %s", timeout, last)
		}
		time.Sleep(interval)
	}
}

// dump writes [v] as indented JSON to <network dir>/api-examples/<name>.json.
func (n *network) dump(name string, v interface{}) {
	dir := filepath.Join(n.dir, "api-examples")
	require.NoError(n.t, os.MkdirAll(dir, 0o750))
	b, err := json.MarshalIndent(v, "", "  ")
	require.NoError(n.t, err)
	require.NoError(n.t, os.WriteFile(filepath.Join(dir, name+".json"), b, 0o600))
}

// evmBlockNumber queries the U-chain's EVM JSON-RPC endpoint.
func evmBlockNumber(uri string) (string, error) {
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`)
	resp, err := http.Post(uri+"/ext/bc/U/rpc", "application/json", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("eth_blockNumber: %s", out.Error.Message)
	}
	return out.Result, nil
}

// rawCall sends a JSON-RPC request and returns the error code, if any.
func rawCall(uri, path, method string) (int, error) {
	body := strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{}}`, method))
	resp, err := http.Post(uri+path, "application/json", body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.Error == nil {
		return 0, nil
	}
	return out.Error.Code, nil
}
