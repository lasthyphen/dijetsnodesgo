// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package genesis

import (
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/sampler"
)

// getIPs returns the beacon IPs for each network
func getIPs(networkID uint32) []string {
	switch networkID {
	case constants.MainnetID:
		return []string{
			"20.108.178.171:9651",
			"20.108.178.171:9653",
			"20.108.178.171:9655",
			"20.108.178.171:9657",
			"20.108.178.171:9659",
		}
	case constants.TahoeID:
		return []string{
			"127.0.0.4:9651",
			"127.0.0.4:9666",
			"127.0.0.4:9678",
			"127.0.0.4:9688",
			"127.0.0.4:9689",
		}
	default:
		return nil
	}
}

// getNodeIDs returns the beacon node IDs for each network
func getNodeIDs(networkID uint32) []string {
	switch networkID {
	case constants.MainnetID:
		return []string{
			"NodeID-LbqzrSMKAssm5Ds2bJ8LPwymnGJbrybCN",
			"NodeID-KnSQaB5tDy1rgZWHFdCHSwTTdNvRNkLBP",
			"NodeID-Nh98zaRumrBz3K5i3SQG1T3WrhJv1Jxmb",
			"NodeID-4tsvZR6LEaVc8mbc5K4Kusji21YZDdk39",
			"NodeID-L7rEYewk8URjAg3kp39zrYcfRk8hH14Y5",
		}
	case constants.TahoeID:
		return []string{
			"NodeID-7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg",
			"NodeID-MFrZFVCXPv5iCn6M9K6XduxGTYp891xXZ",
			"NodeID-NFBbbJ4qCmNaCzeW7sxErhvWqvEQMnYcN",
			"NodeID-GWPcbFJZFfZreETSoWjPimr846mXEKCtu",
			"NodeID-P7oB2McjBGgW2NXXWVYjV8JEDFoW9xDE5",
		}
	default:
		return nil
	}
}

// SampleBeacons returns the some beacons this node should connect to
func SampleBeacons(networkID uint32, count int) ([]string, []string) {
	ips := getIPs(networkID)
	ids := getNodeIDs(networkID)

	if numIPs := len(ips); numIPs < count {
		count = numIPs
	}

	sampledIPs := make([]string, 0, count)
	sampledIDs := make([]string, 0, count)

	s := sampler.NewUniform()
	_ = s.Initialize(uint64(len(ips)))
	indices, _ := s.Sample(count)
	for _, index := range indices {
		sampledIPs = append(sampledIPs, ips[int(index)])
		sampledIDs = append(sampledIDs, ids[int(index)])
	}

	return sampledIPs, sampledIDs
}
