// nodeid prints the node ID of a staking certificate.
package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/lasthyphen/dijetsnodesgo/ids"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: nodeid <staker.crt>")
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		fmt.Fprintln(os.Stderr, "no PEM certificate in", os.Args[1])
		os.Exit(1)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(ids.NodeIDFromCert(cert))
}
