

package dynamicip

import "github.com/lasthyphen/dijetsnodesgo/utils/logging"

var _ Updater = noUpdater{}

func NewNoUpdater() Updater {
	return noUpdater{}
}

type noUpdater struct{}

func (noUpdater) Dispatch(logging.Logger) {}

func (noUpdater) Stop() {}
