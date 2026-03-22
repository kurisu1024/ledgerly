package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurisu1024/ledgerly/service"
)

func main() {
	if err := runService(); err != nil {
		panic(fmt.Errorf("service failed to start: %w", err))
	}
}

func runService() error {
	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Channel to capture service errors
	errChan := make(chan error, 1)

	// Start service in goroutine
	go func() {
		errChan <- service.New().Run(ctx)
	}()

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
		cancel() // Cancel context to trigger graceful shutdown
		// Wait for service to finish shutdown
		return <-errChan
	case err := <-errChan:
		return err
	}
}
