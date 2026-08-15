package stub

import (
	"context"
	"fmt"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

// Stub is a placeholder adapter until real protocol code lands.
type Stub struct {
	id platform.ID
}

func New(id platform.ID) *Stub { return &Stub{id: id} }

func (s *Stub) ID() platform.ID { return s.id }

func (s *Stub) Start(ctx context.Context) (*adapter.StartResult, error) {
	_ = ctx
	return nil, fmt.Errorf("%s: start not implemented yet", s.id)
}

func (s *Stub) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	// idempotent empty stop
	return &adapter.StopResult{
		Platform: string(s.id),
		RoomID:   "",
		Status:   "stopped",
	}, nil
}
