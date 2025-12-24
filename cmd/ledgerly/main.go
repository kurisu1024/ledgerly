package main

import (
	"context"
	"fmt"

	"github.com/kurisu1024/ledgerly/service"
)

func main() {
	if err := runService(); err != nil {
		panic(fmt.Errorf("service failed to start: %w", err))
	}
}

func runService() error {
	return service.New().Run(context.Background())
}
