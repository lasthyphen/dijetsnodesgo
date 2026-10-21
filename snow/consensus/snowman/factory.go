

package snowman

// Factory returns new instances of Consensus
type Factory interface {
	New() Consensus
}
