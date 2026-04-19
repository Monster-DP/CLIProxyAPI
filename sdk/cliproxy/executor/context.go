package executor

import (
	"context"
	"time"
)

type downstreamWebsocketContextKey struct{}
type requestStartContextKey struct{}
type providerFirstByteStartContextKey struct{}

// WithDownstreamWebsocket marks the current request as coming from a downstream websocket connection.
func WithDownstreamWebsocket(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, downstreamWebsocketContextKey{}, true)
}

// DownstreamWebsocket reports whether the current request originates from a downstream websocket connection.
func DownstreamWebsocket(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	raw := ctx.Value(downstreamWebsocketContextKey{})
	enabled, ok := raw.(bool)
	return ok && enabled
}

// WithRequestStart stores the time when the request first entered CLIProxyAPI.
func WithRequestStart(ctx context.Context, startedAt time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if startedAt.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, requestStartContextKey{}, startedAt)
}

// RequestStart returns the request entry time when available.
func RequestStart(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	raw := ctx.Value(requestStartContextKey{})
	startedAt, ok := raw.(time.Time)
	if !ok || startedAt.IsZero() {
		return time.Time{}, false
	}
	return startedAt, true
}

// WithProviderFirstByteStart stores the provider attempt start time used for
// first-byte timeout and usage reporting alignment.
func WithProviderFirstByteStart(ctx context.Context, startedAt time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if startedAt.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, providerFirstByteStartContextKey{}, startedAt)
}

// ProviderFirstByteStart returns the provider attempt start time when available.
func ProviderFirstByteStart(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	raw := ctx.Value(providerFirstByteStartContextKey{})
	startedAt, ok := raw.(time.Time)
	if !ok || startedAt.IsZero() {
		return time.Time{}, false
	}
	return startedAt, true
}
