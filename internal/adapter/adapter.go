package adapter

import (
	"context"

	"github.com/xifan2333/webcast-mate/internal/platform"
)

// StartResult is the fixed stdout payload for `start` (SPEC §5.3).
type StartResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	Cookie   string `json:"cookie"`
	Server   string `json:"server"`
	Key      string `json:"key"`
}

// StopResult is the fixed stdout payload for `stop` (SPEC §5.3).
type StopResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	Status   string `json:"status"` // always "stopped"
}

// StatusResult is stdout for `status` — same core fields as start, plus remote status.
type StatusResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	Cookie   string `json:"cookie"`
	Server   string `json:"server"`
	Key      string `json:"key"`
	// Status from platform query: live | idle | round (bilibili 轮播), etc.
	Status string `json:"status"`
}

// Adapter is the only platform extension point.
type Adapter interface {
	ID() platform.ID
	// Start ensures session, goes live, writes conf side-effects via caller,
	// and returns the JSON fields. Douyin may start a keepalive externally.
	// opts.Yes skips interactive prompts (npm-init style -y).
	Start(ctx context.Context, opts StartOpts) (*StartResult, error)
	// Stop ends the live session. Missing room must return success (idempotent).
	Stop(ctx context.Context) (*StopResult, error)
	// Status queries the platform for live state; fills cookie/server/key from local when known.
	Status(ctx context.Context) (*StatusResult, error)
}

// Registry maps platform id → adapter.
type Registry struct {
	byID map[platform.ID]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{byID: make(map[platform.ID]Adapter, len(adapters))}
	for _, a := range adapters {
		r.byID[a.ID()] = a
	}
	return r
}

func (r *Registry) Get(id platform.ID) (Adapter, bool) {
	a, ok := r.byID[id]
	return a, ok
}

func (r *Registry) IDs() []platform.ID {
	out := make([]platform.ID, 0, len(r.byID))
	for _, id := range platform.All {
		if _, ok := r.byID[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
