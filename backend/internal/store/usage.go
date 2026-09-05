package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// UsageEvent is one metered protocol operation.
type UsageEvent struct {
	AppID       uuid.UUID
	SubjectHash string
	Kind        string
	Curve       string
	KeyID       string
	NodeAddress string
	LatencyMS   int
	OK          bool
	ErrorCode   string
	OccurredAt  time.Time
}

// RecordUsage appends metering events and refreshes the affected daily
// rollups in one transaction, so the analytics screen never shows a day whose
// rollup lags its own raw events.
func (s *Store) RecordUsage(ctx context.Context, events []UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	affected := map[string]struct {
		app uuid.UUID
		day time.Time
	}{}

	for _, e := range events {
		at := e.OccurredAt
		if at.IsZero() {
			at = time.Now()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO usage_events
				(app_id, subject_hash, kind, curve, key_id, node_address, latency_ms, ok, error_code, occurred_at)
			VALUES ($1, NULLIF($2,''), $3, NULLIF($4,''), NULLIF($5,''), lower(NULLIF($6,'')),
			        NULLIF($7,0), $8, NULLIF($9,''), $10)`,
			e.AppID, e.SubjectHash, e.Kind, e.Curve, e.KeyID, e.NodeAddress,
			e.LatencyMS, e.OK, e.ErrorCode, at); err != nil {
			return err
		}
		day := at.UTC().Truncate(24 * time.Hour)
		affected[e.AppID.String()+day.Format("2006-01-02")] = struct {
			app uuid.UUID
			day time.Time
		}{e.AppID, day}
	}

	for _, a := range affected {
		if err := rollupDay(ctx, tx, a.app, a.day); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// txExec is the exec surface shared by *pgxpool.Pool and pgx.Tx, so the
// rollup can run either inside the caller's transaction or on its own.
type txExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// rollupDay recomputes one app-day rollup from its raw events.
//
// Active wallets is a distinct count, not a sum, so it must be recomputed
// rather than incremented — a user who signs twice in a day is one active
// wallet, and that is precisely the unit the network bills on.
func rollupDay(ctx context.Context, tx txExec, appID uuid.UUID, day time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO usage_daily
			(app_id, day, active_wallets, auth_count, keygen_count, sign_count, error_count,
			 p50_latency_ms, p95_latency_ms)
		SELECT
			$1, $2::date,
			count(DISTINCT subject_hash) FILTER (WHERE subject_hash IS NOT NULL),
			count(*) FILTER (WHERE kind = 'auth'),
			count(*) FILTER (WHERE kind = 'keygen'),
			count(*) FILTER (WHERE kind = 'sign'),
			count(*) FILTER (WHERE NOT ok),
			percentile_disc(0.5)  WITHIN GROUP (ORDER BY latency_ms)::int,
			percentile_disc(0.95) WITHIN GROUP (ORDER BY latency_ms)::int
		FROM usage_events
		WHERE app_id = $1 AND occurred_at >= $2::date AND occurred_at < $2::date + interval '1 day'
		ON CONFLICT (app_id, day) DO UPDATE SET
			active_wallets = EXCLUDED.active_wallets,
			auth_count     = EXCLUDED.auth_count,
			keygen_count   = EXCLUDED.keygen_count,
			sign_count     = EXCLUDED.sign_count,
			error_count    = EXCLUDED.error_count,
			p50_latency_ms = EXCLUDED.p50_latency_ms,
			p95_latency_ms = EXCLUDED.p95_latency_ms`,
		appID, day)
	return err
}

// Usage returns the analytics series and headline figures for a date range.
func (s *Store) Usage(ctx context.Context, appID uuid.UUID, from, to time.Time) (*structs.UsageSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT day::text, active_wallets, auth_count, keygen_count, sign_count,
		       error_count, p50_latency_ms, p95_latency_ms
		FROM usage_daily
		WHERE app_id=$1 AND day >= $2::date AND day <= $3::date
		ORDER BY day`, appID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sum := &structs.UsageSummary{
		From:   from.Format("2006-01-02"),
		To:     to.Format("2006-01-02"),
		Series: []structs.UsagePoint{},
	}
	var totalOps, totalErrors int
	for rows.Next() {
		var p structs.UsagePoint
		if err := rows.Scan(&p.Day, &p.ActiveWallets, &p.AuthCount, &p.KeygenCount,
			&p.SignCount, &p.ErrorCount, &p.P50LatencyMS, &p.P95LatencyMS); err != nil {
			return nil, err
		}
		sum.Series = append(sum.Series, p)
		sum.TotalSigns += p.SignCount
		sum.TotalKeygens += p.KeygenCount
		sum.TotalAuths += p.AuthCount
		totalOps += p.AuthCount + p.KeygenCount + p.SignCount
		totalErrors += p.ErrorCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if totalOps > 0 {
		sum.ErrorRate = float64(totalErrors) / float64(totalOps)
	}

	// Distinct wallets over the whole window, which is the billed figure —
	// summing the daily counts would double-count returning users.
	if err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT subject_hash)
		FROM usage_events
		WHERE app_id=$1 AND subject_hash IS NOT NULL
		  AND occurred_at >= $2::date AND occurred_at < $3::date + interval '1 day'`,
		appID, from, to).Scan(&sum.MonthlyActiveWallets); err != nil {
		return nil, err
	}
	return sum, nil
}

// OrgMonthlyActiveWallets counts distinct active wallets across an org's apps
// for a period — the unit the network's economics price on.
func (s *Store) OrgMonthlyActiveWallets(ctx context.Context, orgID uuid.UUID, from, to time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT (e.app_id::text || ':' || e.subject_hash))
		FROM usage_events e
		JOIN apps a ON a.id = e.app_id
		WHERE a.org_id = $1 AND e.subject_hash IS NOT NULL
		  AND e.occurred_at >= $2::date AND e.occurred_at < $3::date + interval '1 day'`,
		orgID, from, to).Scan(&n)
	return n, err
}

// RollupDay recomputes one app-day rollup outside a transaction, for the
// nightly backfill job.
func (s *Store) RollupDay(ctx context.Context, appID uuid.UUID, day time.Time) error {
	return rollupDay(ctx, s.pool, appID, day)
}
