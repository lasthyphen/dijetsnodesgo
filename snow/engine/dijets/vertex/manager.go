

package vertex

// Manager defines all the vertex related functionality that is required by the
// consensus engine.
type Manager interface {
	Builder
	Parser
	Storage
}
