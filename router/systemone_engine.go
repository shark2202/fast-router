package router

import (
	"context"
	"errors"
)

// SystemOneEngine is the high-level decision boundary used by the gateway.
// Implementations may be in-process native scorers or remote HTTP services.
type SystemOneEngine interface {
	Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}

// ErrSystemOneEngineUnavailable indicates that an engine was requested but
// has not been initialized.
var ErrSystemOneEngineUnavailable = errors.New("system-one engine unavailable")

// NativeSystemOneEngine adapts the existing in-process Scorer to the
// replaceable SystemOneEngine boundary.
type NativeSystemOneEngine struct {
	scorer *Scorer
}

func NewNativeSystemOneEngine(scorer *Scorer) *NativeSystemOneEngine {
	return &NativeSystemOneEngine{scorer: scorer}
}

func (e *NativeSystemOneEngine) Evaluate(ctx context.Context, req System1Request) (System1Response, error) {
	if err := ctx.Err(); err != nil {
		return System1Response{}, err
	}
	if e == nil || e.scorer == nil {
		return System1Response{}, ErrSystemOneEngineUnavailable
	}
	resp, err := e.scorer.Score(req)
	if err != nil {
		return System1Response{}, err
	}
	if err := ctx.Err(); err != nil {
		return System1Response{}, err
	}
	return resp, nil
}
