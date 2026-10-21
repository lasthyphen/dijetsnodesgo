

package snowstorm

import (
	"testing"
)

func TestDirectedConsensus(t *testing.T) {
	runConsensusTests(t, DirectedFactory{}, "DG")
}
