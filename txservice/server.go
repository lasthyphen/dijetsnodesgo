package txservice

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/rpc/v2"

	"github.com/lasthyphen/dijetsnodesgo/utils/json"
)

const maxRequestBytes = 64 * 1024

// NewHandler returns the HTTP handler serving the payments.* API at
// Endpoint. Every request must carry "Authorization: Bearer <token>".
func NewHandler(s *Service) (http.Handler, error) {
	server := rpc.NewServer()
	server.RegisterCodec(json.NewCodec(), "application/json")
	server.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")
	if err := server.RegisterService(s, ServiceName); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle(Endpoint, s.authenticate(server))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux, nil
}

// NewServer returns an HTTP server for [handler] with conservative timeouts.
// Submit waits for the network's decision, so the write timeout leaves room
// for the decision timeout.
func NewServer(config *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              config.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      time.Duration(config.DecisionTimeout) + 30*time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func (s *Service) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if client, ok := s.config.authenticate(token); ok {
			r = r.WithContext(withCaller(r.Context(), client))
		}
		// Unauthenticated requests still reach the service, which answers
		// them with a JSON-RPC "unauthorized" error.
		next.ServeHTTP(w, r)
	})
}
