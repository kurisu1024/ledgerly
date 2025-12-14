package main

import "fmt"

func main() {
	if err := runService(); err != nil {
		panic(fmt.Errorf("service failed to start: %w", err))
	}
}

func runService() error {
	fmt.Printf("Place holder for running service. \n")
	return nil
}
