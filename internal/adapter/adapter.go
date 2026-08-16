package adapter

import (
	"context"

	"github.com/xifan2333/webcast-mate/internal/platform"
)

// AuthBuckets is requests-style auth on stdout (same as secrets file).
type AuthBuckets struct {
	Cookies map[string]string `json:"cookies,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
}

type StartResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	AuthBuckets
	Server string `json:"server"`
	Key    string `json:"key"`
}

type StopResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	Status   string `json:"status"`
}

type StatusResult struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room_id"`
	AuthBuckets
	Server string `json:"server"`
	Key    string `json:"key"`
	Status string `json:"status"`
}

type LoginResult struct {
	Platform string `json:"platform"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	AuthBuckets
	LoginAt string `json:"login_at,omitempty"`
}

type LogoutResult struct {
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

type Adapter interface {
	ID() platform.ID
	Login(ctx context.Context) (*LoginResult, error)
	Logout(ctx context.Context) (*LogoutResult, error)
	Start(ctx context.Context, opts StartOpts) (*StartResult, error)
	Stop(ctx context.Context) (*StopResult, error)
	Status(ctx context.Context) (*StatusResult, error)
}

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
