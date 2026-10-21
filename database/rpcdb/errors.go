

package rpcdb

import (
	"github.com/lasthyphen/dijetsnodesgo/database"
)

var (
	errCodeToError = map[uint32]error{
		1: database.ErrClosed,
		2: database.ErrNotFound,
	}
	errorToErrCode = map[error]uint32{
		database.ErrClosed:   1,
		database.ErrNotFound: 2,
	}
)

func errorToRPCError(err error) error {
	if _, ok := errorToErrCode[err]; ok {
		return nil
	}
	return err
}
