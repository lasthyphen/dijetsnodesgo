package minichain

import (
	"net/http"

	"github.com/gorilla/rpc/v2"

	"github.com/lasthyphen/dijetsnodesgo/utils/json"
)

// ServiceName is the JSON-RPC service name: methods are "minichain.<method>".
const ServiceName = "minichain"

// Endpoint is the extension under which the service is mounted on the
// M-chain: /ext/bc/M/minichain.
const Endpoint = "/minichain"

// NewHandler returns the JSON-RPC handler serving [s].
func NewHandler(s *Service) (http.Handler, error) {
	server := rpc.NewServer()
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")
	if interceptor := s.observer.apiInterceptor; interceptor != nil {
		server.RegisterInterceptFunc(interceptor.InterceptRequest)
		server.RegisterAfterFunc(interceptor.AfterRequest)
	}
	return server, server.RegisterService(s, ServiceName)
}
