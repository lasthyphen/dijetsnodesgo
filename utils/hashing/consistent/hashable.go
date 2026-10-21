

package consistent

// Hashable is an interface to be implemented by structs that need to be sharded via consistent hashing.
type Hashable interface {
	// ConsistentHashKey is the key used to shard the blob.
	// This should be constant for a given blob.
	ConsistentHashKey() []byte
}
