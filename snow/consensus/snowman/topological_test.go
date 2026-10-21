

package snowman

import (
	"testing"
)

func TestTopological(t *testing.T) {
	runConsensusTests(t, TopologicalFactory{})
}
