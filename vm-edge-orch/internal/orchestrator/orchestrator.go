package common

import (
	"context"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/config"
	impl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/orchestrator/imp"
)

// Orchestrator defines the lifecycle for the orchestrator and access to services.
// Implementations should be fully initialised via Init before Start is called.
type Orchestrator interface {
	Init(cfg *config.Config) error
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// New creates a new Orchestrator implementation backed by the imp.Server.
func New() Orchestrator {
	return impl.NewServer()
}
