package requeststate

import (
	"context"
	"time"
)

type (
	key   struct{}
	State struct {
		Context           context.Context
		Method, Route     string
		Started           time.Time
		Status            int
		Code              string
		Err, WriteError   error
		Stack             []byte
		Panic             any
		Aborted, Prepared bool
		OnMetrics, OnLog  func()
		measured, logged  bool
	}
)

func With(ctx context.Context, state *State) context.Context {
	return context.WithValue(ctx, key{}, state)
}
func From(ctx context.Context) *State { state, _ := ctx.Value(key{}).(*State); return state }
func Fail(ctx context.Context, status int, code string, err error, stack []byte) {
	if state := From(ctx); state != nil {
		state.Status = status
		state.Code = code
		state.Err = err
		if len(stack) > 0 {
			state.Stack = stack
		}
	}
}

func (s *State) FinishMetrics() {
	if !s.measured && s.OnMetrics != nil {
		s.measured = true
		s.OnMetrics()
	}
}

func (s *State) FinishLog() {
	if !s.logged && s.OnLog != nil {
		s.logged = true
		s.OnLog()
	}
}
