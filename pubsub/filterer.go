

package pubsub

type Filterer interface {
	Filter(connections []Filter) ([]bool, interface{})
}
