// Package health implements the readiness check.
package health

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AssetPinger verifies object storage is reachable. It is nil in development,
// where uploads go to the local filesystem and there is nothing to ping.
type AssetPinger func(ctx context.Context) error

// Status is the outcome of a readiness check.
type Status struct {
	OK     bool   `json:"ok"`
	Failed string `json:"failed,omitempty"`
	Err    error  `json:"-"`
	Detail string `json:"detail,omitempty"`
}

// Check verifies every dependency the server cannot serve traffic without.
//
// It deliberately excludes the chain RPC and the node fleet: the platform is
// still useful — and should still report ready — when a node is down or an RPC
// endpoint is flaky, because those are exactly the conditions the console
// exists to show a developer.
func Check(ctx context.Context, pool *pgxpool.Pool, assets AssetPinger) Status {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return Status{Failed: "database", Err: err, Detail: err.Error()}
	}
	if assets != nil {
		if err := assets(ctx); err != nil {
			return Status{Failed: "assets", Err: err, Detail: err.Error()}
		}
	}
	return Status{OK: true}
}
