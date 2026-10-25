

package genesis

import (
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/sampler"
)

// getIPs returns the beacon IPs for each network
func getIPs(networkID uint32) []string {
	switch networkID {
	case constants.MainnetID:
		// The genesis validators, in the order of getNodeIDs, as run by
		// scripts/run_mainnet_cluster.sh on one host (staking ports
		// 9651, 9653, ..., 9659).
		return []string{
			"20.108.178.171:9651",
			"20.108.178.171:9653",
			"20.108.178.171:9655",
			"20.108.178.171:9657",
			"20.108.178.171:9659",
		}
	case constants.LotharID:
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
			"NodeID-FU6TByNdgJhbQF8WeFwfKuD3QnMcQJMjP",
			"NodeID-MxGY1zYcaP9CM1LvW9RKBDZfbZa5dBSK3",
			"NodeID-HTdNxjm2cKe1hUzdHBiEvA3zr4wYj3Vg9",
			"NodeID-5nJLrvj92ztUAGQrfcPHzuHdm5X9ykeRd",
			"NodeID-71qPRtPrLBcaHrAMKF39tNrExJWhfaKpr",
		}
	case constants.LotharID:
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
