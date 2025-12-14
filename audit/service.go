package audit

import "context"

// TODO: define service data structure and interface.
type Service interface {
	Run(ctx context.Context) error
}

type service struct{}

// TODO: Implement
func Run(ctx context.Context) error { return nil }
