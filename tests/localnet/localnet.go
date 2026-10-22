// Package localnet runs a real 5-node staking network on this machine for
// end-to-end tests. Build the node first:
//
//	./scripts/build_dijets.sh
package localnet

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
	NetworkID     = 1337
	NumNodes      = 5
	bootstrapWait = 5 * time.Minute
)

// Options distinguish the networks of different test suites, so that they
// never share ports or directories.
type Options struct {
	// Name prefixes the network directory and the genesis message.
	Name string
	// BasePort is node1's HTTP port. Node i uses BasePort+2(i-1) for HTTP
	// and the next port for staking.
	BasePort int
}

// Node IDs of staking/local/staker{1..5}.crt, in order.
var stakerNodeIDs = []string{
	"NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg",
	"NodeID-MFrZFVCXPv5iCn6M9K6XduxGTYp891xXZ",
	"NodeID-NFBbbJ4qCmNaCzeW7sxErhvWqvEQMnYcN",
	"NodeID-GWPcbFJZFfZreETSoWjPimr846mXEKCtu",
	"NodeID-P7oB2McjBGgW2NXXWVYjV8JEDFoW9xDE5",
}

type Node struct {
	Idx         int
	NodeID      ids.NodeID
	httpPort    int
	stakingPort int
	DataDir     string
	cmd         *exec.Cmd
	exited      chan struct{}
	// Whitelist lists the subnets the node validates, applied on its next
	// start.
	Whitelist []ids.ID
}

func (n *Node) URI() string { return fmt.Sprintf("http://127.0.0.1:%d", n.httpPort) }

func (n *Node) Running() bool {
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

type Network struct {
	t           *testing.T
	options     Options
	repoRoot    string
	binary      string
	Dir         string
	genesisPath string
	pluginDir   string
	Nodes       []*Node
}

func repoRoot(t *testing.T) string {
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// New prepares, but doesn't start, a network. The node binary is
// $E2E_NODE_BINARY, $ARC_NODE_BINARY or build/dijetsnodesgo.
func New(t *testing.T, options Options) *Network {
	root := repoRoot(t)
	binary := os.Getenv("E2E_NODE_BINARY")
	if binary == "" {
		binary = os.Getenv("ARC_NODE_BINARY")
	}
	if binary == "" {
		binary = filepath.Join(root, "build", "dijetsnodesgo")
	}
	_, err := os.Stat(binary)
	require.NoError(t, err, "build the node first: ./scripts/build_dijets.sh")

	dir, err := os.MkdirTemp("", options.Name+"-e2e-")
	require.NoError(t, err)
	t.Logf("network directory (kept for inspection): %s", dir)

	n := &Network{
		t:         t,
		options:   options,
		repoRoot:  root,
		binary:    binary,
		Dir:       dir,
		pluginDir: filepath.Join(dir, "plugins"),
	}
	require.NoError(t, os.MkdirAll(n.pluginDir, 0o750))
	n.writeGenesis()

	for i := 0; i < NumNodes; i++ {
		nodeID, err := ids.NodeIDFromString(stakerNodeIDs[i])
		require.NoError(t, err)
		n.Nodes = append(n.Nodes, &Node{
			Idx:         i + 1,
			NodeID:      nodeID,
			httpPort:    options.BasePort + 2*i,
			stakingPort: options.BasePort + 2*i + 1,
			DataDir:     filepath.Join(dir, fmt.Sprintf("node%d", i+1)),
		})
	}
	t.Cleanup(n.StopAll)
	return n
}

// writeGenesis derives a custom-network genesis from the local genesis with a
// recent start time; the shipped local genesis' staking periods have expired.
func (n *Network) writeGenesis() {
	raw, err := os.ReadFile(filepath.Join(n.repoRoot, "genesis", "genesis_local.json"))
	require.NoError(n.t, err)
	var g map[string]interface{}
	require.NoError(n.t, json.Unmarshal(raw, &g))
	g["networkID"] = NetworkID
	g["startTime"] = time.Now().Add(-time.Minute).Unix()
	g["message"] = n.options.Name + " e2e"
	out, err := json.MarshalIndent(g, "", "  ")
	require.NoError(n.t, err)
	n.genesisPath = filepath.Join(n.Dir, "genesis.json")
	require.NoError(n.t, os.WriteFile(n.genesisPath, out, 0o600))
}

// Node returns node [i], counting from 1.
func (n *Network) Node(i int) *Node { return n.Nodes[i-1] }

// Start starts a node without waiting for it to bootstrap.
func (n *Network) Start(nd *Node, extraArgs ...string) {
	args := []string{
		fmt.Sprintf("--network-id=%d", NetworkID),
		"--genesis=" + n.genesisPath,
		"--data-dir=" + nd.DataDir,
		// Plugins are loaded from <build-dir>/plugins; use an empty one. The
		// U-chain EVM is compiled into the node.
		"--build-dir=" + n.Dir,
		"--http-host=127.0.0.1",
		fmt.Sprintf("--http-port=%d", nd.httpPort),
		fmt.Sprintf("--staking-port=%d", nd.stakingPort),
		"--public-ip=127.0.0.1",
		fmt.Sprintf("--staking-tls-cert-file=%s", filepath.Join(n.repoRoot, "staking", "local", fmt.Sprintf("staker%d.crt", nd.Idx))),
		fmt.Sprintf("--staking-tls-key-file=%s", filepath.Join(n.repoRoot, "staking", "local", fmt.Sprintf("staker%d.key", nd.Idx))),
		"--log-level=info",
		"--log-display-level=warn",
		"--health-check-frequency=2s",
	}
	if nd.Idx != 1 {
		first := n.Nodes[0]
		args = append(args,
			fmt.Sprintf("--bootstrap-ips=127.0.0.1:%d", first.stakingPort),
			"--bootstrap-ids="+first.NodeID.String(),
		)
	}
	if len(nd.Whitelist) > 0 {
		subnets := make([]string, len(nd.Whitelist))
		for i, s := range nd.Whitelist {
			subnets[i] = s.String()
		}
		args = append(args, "--whitelisted-subnets="+strings.Join(subnets, ","))
	}
	args = append(args, extraArgs...)

	require.NoError(n.t, os.MkdirAll(nd.DataDir, 0o750))
	logFile, err := os.OpenFile(filepath.Join(nd.DataDir, "stdout.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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
	n.t.Logf("started node%d (%s) whitelist=%v", nd.Idx, nd.NodeID, nd.Whitelist)
}

func (n *Network) Stop(nd *Node) {
	if !nd.Running() {
		return
	}
	_ = nd.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-nd.exited:
	case <-time.After(60 * time.Second):
		_ = nd.cmd.Process.Kill()
		<-nd.exited
	}
	n.t.Logf("stopped node%d", nd.Idx)
}

func (n *Network) StopAll() {
	for _, nd := range n.Nodes {
		n.Stop(nd)
	}
}

// StartAll starts every node and waits for the primary network chains to
// bootstrap.
func (n *Network) StartAll(extraArgs ...string) {
	for _, nd := range n.Nodes {
		n.Start(nd, extraArgs...)
	}
	for _, nd := range n.Nodes {
		n.AwaitBootstrapped(nd, "M", "V", "U")
	}
}

func (n *Network) RestartAll(extraArgs ...string) {
	n.StopAll()
	n.StartAll(extraArgs...)
}

func (n *Network) AwaitBootstrapped(nd *Node, chains ...string) {
	client := info.NewClient(nd.URI())
	Eventually(n.t, bootstrapWait, time.Second, func() (bool, string) {
		if !nd.Running() {
			n.t.Fatalf("node%d exited; see %s", nd.Idx, filepath.Join(nd.DataDir, "stdout.log"))
		}
		for _, chain := range chains {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			ok, err := client.IsBootstrapped(ctx, chain)
			cancel()
			if err != nil || !ok {
				return false, fmt.Sprintf("node%d chain %s not bootstrapped (err=%v)", nd.Idx, chain, err)
			}
		}
		return true, ""
	})
}

// Eventually polls [cond] until it holds, failing the test after [timeout].
func Eventually(t *testing.T, timeout, interval time.Duration, cond func() (bool, string)) {
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

// Dump writes [v] as indented JSON to <network dir>/api-examples/<name>.json.
func (n *Network) Dump(name string, v interface{}) {
	dir := filepath.Join(n.Dir, "api-examples")
	require.NoError(n.t, os.MkdirAll(dir, 0o750))
	b, err := json.MarshalIndent(v, "", "  ")
	require.NoError(n.t, err)
	require.NoError(n.t, os.WriteFile(filepath.Join(dir, name+".json"), b, 0o600))
}

// EVMBlockNumber queries the U-chain's EVM JSON-RPC endpoint.
func EVMBlockNumber(uri string) (string, error) {
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

// RawCall sends a JSON-RPC request and returns the error code, if any.
func RawCall(uri, path, method string) (int, error) {
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
