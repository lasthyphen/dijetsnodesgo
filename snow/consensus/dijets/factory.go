

package dijets

// Factory returns new instances of Consensus
type Factory interface {
	New() Consensus
}
