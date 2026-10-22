// Command txservice runs the trusted transaction service.
//
//	txservice -config txservice.json
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/txservice"
)

func main() {
	configPath := flag.String("config", "txservice.json", "path to the configuration file")
	flag.Parse()

	logger := log.New(os.Stderr, "txservice ", log.LstdFlags|log.LUTC)
	config, err := txservice.LoadConfig(*configPath)
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	network, err := txservice.NewNodeNetwork(ctx, config.NodeURI)
	cancel()
	if err != nil {
		logger.Fatalf("failed to connect to %s: %v", config.NodeURI, err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	service, err := txservice.NewService(ctx, config, network, logger, time.Now)
	cancel()
	if err != nil {
		logger.Fatalf("failed to start: %v", err)
	}
	defer service.Close()
	for _, d := range config.Delegations {
		addr, _ := service.AgentAddress(d.ID)
		logger.Printf("delegation %q: agent %s", d.ID, addr)
	}

	handler, err := txservice.NewHandler(service)
	if err != nil {
		logger.Fatalf("failed to create handler: %v", err)
	}
	server := txservice.NewServer(config, handler)

	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		<-signals
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	logger.Printf("serving %s on %s", txservice.Endpoint, config.Listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server failed: %v", err)
	}
}
