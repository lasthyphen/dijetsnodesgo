

package utils

// Returns a new instance of a T.
func Zero[T any]() T {
	return *new(T) //nolint:gocritic
}
