// Command arcctl creates Arcs from an Arc Studio specification by running
// the node's own creation workflow (arc/workflow) with a key held locally.
//
//	arcctl validate -spec arc.json [-node http://127.0.0.1:9650]
//	arcctl create   -spec arc.json -key-file owner.key [-node …] [-out result.json]
//
// validate checks the specification offline and against the node (network
// ID, committee eligibility) without signing anything. create signs and
// issues CreateSubnetTx, AddSubnetValidatorTx for each member and
// CreateChainTx, waiting for each. It writes what it did to -out, so a
// failed run can be retried with the subnet ID it already created.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/api/info"
	"github.com/lasthyphen/dijetsnodesgo/arc"
	"github.com/lasthyphen/dijetsnodesgo/arc/workflow"
	"github.com/lasthyphen/dijetsnodesgo/genesis"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
	wcommon "github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"
)

const usage = `usage:
  arcctl validate -spec arc.json [-node URI]
  arcctl create   -spec arc.json -key-file owner.key [-node URI] [-out result.json]`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	specPath := fs.String("spec", "", "Arc Studio specification (JSON)")
	node := fs.String("node", "http://127.0.0.1:9650", "node URI")
	keyFile := fs.String("key-file", "", "file holding a control address's private key (PrivateKey-…)")
	out := fs.String("out", "", "where to write the result (default: <spec>.result.json)")
	timeout := fs.Duration("timeout", 30*time.Minute, "give up after this long")
	_ = fs.Parse(os.Args[2:])
	if *specPath == "" {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var err error
	switch cmd {
	case "validate":
		err = validate(ctx, *specPath, *node)
	case "create":
		o := *out
		if o == "" {
			o = strings.TrimSuffix(*specPath, ".json") + ".result.json"
		}
		err = create(ctx, *specPath, *node, *keyFile, o)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "arcctl:", err)
		os.Exit(1)
	}
}

// prepare reads the spec, builds the request and runs every offline check,
// then confirms the node is on the spec's network.
func prepare(ctx context.Context, specPath, node string) (*Spec, workflow.Request, error) {
	s, err := readSpec(specPath)
	if err != nil {
		return nil, workflow.Request{}, err
	}
	req, err := s.request(time.Now())
	if err != nil {
		return s, req, err
	}
	if err := workflow.ValidateRequest(req); err != nil {
		return s, req, err
	}
	netID, err := info.NewClient(node).GetNetworkID(ctx)
	if err != nil {
		return s, req, fmt.Errorf("couldn't reach %s: %w", node, err)
	}
	if netID != s.NetworkID {
		return s, req, fmt.Errorf("the spec is for network %d but %s is on network %d", s.NetworkID, node, netID)
	}
	return s, req, nil
}

func validate(ctx context.Context, specPath, node string) error {
	s, req, err := prepare(ctx, specPath, node)
	if err != nil {
		return err
	}
	// Committee eligibility, as the workflow checks it before signing.
	nodeIDs := make([]ids.NodeID, 0, len(req.Committee))
	for _, m := range req.Committee {
		nodeIDs = append(nodeIDs, m.NodeID)
	}
	current, err := platformvm.NewClient(node).GetCurrentValidators(ctx, constants.PrimaryNetworkID, nodeIDs)
	if err != nil {
		return fmt.Errorf("couldn't read primary validators: %w", err)
	}
	byID := make(map[ids.NodeID]platformvm.ClientPermissionlessValidator, len(current))
	for _, v := range current {
		byID[v.NodeID] = v
	}
	var problems []string
	for _, m := range req.Committee {
		v, ok := byID[m.NodeID]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s is not a current primary validator", m.NodeID))
		case uint64(req.ValidationEnd.Unix()) > v.EndTime:
			problems = append(problems, fmt.Sprintf("%s stops validating the primary network at %s, before the committee would end", m.NodeID, time.Unix(int64(v.EndTime), 0).UTC().Format(time.RFC3339)))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	fees := genesis.GetTxFeeConfig(s.NetworkID)
	total := uint64(len(req.Committee)) * fees.AddSubnetValidatorFee
	if req.SubnetID == ids.Empty {
		total += fees.CreateSubnetTxFee
	}
	total += fees.CreateBlockchainTxFee
	fmt.Printf("%q is valid for network %d.\n", s.Name, s.NetworkID)
	fmt.Printf("  genesis: %d bytes, sha256 %s\n", len(req.Genesis), arc.FormatGenesisHash(arc.GenesisHash(req.Genesis)))
	fmt.Printf("  committee: %d members, %s to %s\n", len(req.Committee), req.ValidationStart.UTC().Format(time.RFC3339), req.ValidationEnd.UTC().Format(time.RFC3339))
	fmt.Printf("  fees: %s DJT\n", djt(total))
	return nil
}

// Result is what create writes: enough to follow the Arc and to retry.
type Result struct {
	Name           string   `json:"name"`
	NetworkID      uint32   `json:"networkID"`
	SubnetID       string   `json:"subnetID,omitempty"`
	ChainID        string   `json:"chainID,omitempty"`
	ValidatorTxIDs []string `json:"validatorTxIDs,omitempty"`
	CreatedSubnet  bool     `json:"createdSubnet"`
	CreatedChain   bool     `json:"createdChain"`
	Error          string   `json:"error,omitempty"`
	At             string   `json:"at"`
}

func create(ctx context.Context, specPath, node, keyFile, outPath string) error {
	if keyFile == "" {
		return errors.New("create needs -key-file")
	}
	key, err := readKey(keyFile)
	if err != nil {
		return err
	}
	s, req, err := prepare(ctx, specPath, node)
	if err != nil {
		return err
	}
	wallet, err := primary.NewWalletFromURI(ctx, node, secp256k1fx.NewKeychain(key))
	if err != nil {
		return fmt.Errorf("couldn't build wallet: %w", err)
	}
	fmt.Printf("Creating %q on network %d. Each transaction is accepted on its own; this takes a few minutes.\n", s.Name, s.NetworkID)
	res, runErr := workflow.Create(ctx, wallet.P(), platformvm.NewClient(node), arc.NewClient(node), req, wcommon.WithContext(ctx))

	r := Result{Name: s.Name, NetworkID: s.NetworkID, CreatedSubnet: res.CreatedSubnet, CreatedChain: res.CreatedChain, At: time.Now().UTC().Format(time.RFC3339)}
	if res.SubnetID != ids.Empty {
		r.SubnetID = res.SubnetID.String()
	}
	if res.ChainID != ids.Empty {
		r.ChainID = res.ChainID.String()
	}
	for _, id := range res.ValidatorTxIDs {
		r.ValidatorTxIDs = append(r.ValidatorTxIDs, id.String())
	}
	if runErr != nil {
		r.Error = runErr.Error()
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	if err := os.WriteFile(outPath, append(b, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "arcctl: couldn't write result:", err)
	}

	if runErr != nil {
		if r.SubnetID != "" {
			fmt.Printf("Stopped after creating subnet %s. Add \"subnetID\": %q to the spec's subnet block to retry without creating another.\n", r.SubnetID, r.SubnetID)
		}
		return runErr
	}
	fmt.Printf("\nRegistered.\n  chain ID:  %s\n  subnet ID: %s\n  result:    %s\n", r.ChainID, r.SubnetID, outPath)
	fmt.Printf("\nNext: each committee operator adds %s to --whitelisted-subnets and restarts.\n", r.SubnetID)
	fmt.Println("Paste the chain ID into Arc Studio (Review → Follow) to watch it become active.")
	return nil
}

// readKey reads "PrivateKey-…" from a file and refuses files others can read.
func readKey(path string) (*crypto.PrivateKeySECP256K1R, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by other users; run chmod 600 %s", path, path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	k := &crypto.PrivateKeySECP256K1R{}
	// The key's text form is its JSON string without the quotes.
	if err := k.UnmarshalJSON([]byte(`"` + strings.TrimSpace(string(b)) + `"`)); err != nil {
		return nil, fmt.Errorf("%s does not hold a PrivateKey-… key", path)
	}
	return k, nil
}

func djt(n uint64) string {
	whole, frac := n/units.Djtx, n%units.Djtx
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%09d", whole, frac), "0")
}
