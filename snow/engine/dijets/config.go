

package dijets

import (
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/snow/consensus/dijets"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/dijets/vertex"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"
	"github.com/lasthyphen/dijetsnodesgo/snow/validators"
)

// Config wraps all the parameters needed for an dijets engine
type Config struct {
	Ctx *snow.ConsensusContext
	common.AllGetsServer
	VM         vertex.DAGVM
	Manager    vertex.Manager
	Sender     common.Sender
	Validators validators.Set

	Params    dijets.Parameters
	Consensus dijets.Consensus
}
