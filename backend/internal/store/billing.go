package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/oleary-labs/signet-platform/backend/internal/structs"
)

// BillingAccount reads an org's balance and rate, and projects the current
// month's cost from actual usage.
//
// Payments are not switched on: nothing in this codebase debits the balance.
// The projection is shown so a developer can see what the metered usage would
// cost at the published rate, and PaymentsEnabled reports the state plainly
// rather than letting the screen imply money is moving.
func (s *Store) BillingAccount(ctx context.Context, orgID uuid.UUID, paymentsEnabled bool) (*structs.BillingAccount, error) {
	var b structs.BillingAccount
	err := s.pool.QueryRow(ctx, `
		INSERT INTO billing_accounts (org_id) VALUES ($1)
		ON CONFLICT (org_id) DO UPDATE SET org_id = EXCLUDED.org_id
		RETURNING currency, balance_micros, rate_micros_per_maw, low_balance_micros,
		          contract_address, chain_id, status, updated_at`, orgID,
	).Scan(&b.Currency, &b.BalanceMicros, &b.RateMicrosPerMAW, &b.LowBalanceMicros,
		&b.ContractAddress, &b.ChainID, &b.Status, &b.UpdatedAt)
	if err != nil {
		return nil, wrap(err)
	}
	b.PaymentsEnabled = paymentsEnabled

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	maw, err := s.OrgMonthlyActiveWallets(ctx, orgID, monthStart, now)
	if err != nil {
		return nil, err
	}
	b.EstimatedMonthMAW = maw
	b.EstimatedMonthCost = int64(maw) * b.RateMicrosPerMAW
	return &b, nil
}

// UpdateBillingSettings edits the fields an org owner controls. The rate is
// deliberately not among them: it is a protocol-level parameter, not something
// a customer sets for themselves.
func (s *Store) UpdateBillingSettings(ctx context.Context, orgID uuid.UUID, contractAddress string, chainID *int64, lowBalanceMicros *int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE billing_accounts SET
			contract_address   = COALESCE(lower(NULLIF($2,'')), contract_address),
			chain_id           = COALESCE($3, chain_id),
			low_balance_micros = COALESCE($4, low_balance_micros),
			updated_at         = now()
		WHERE org_id=$1`, orgID, contractAddress, chainID, lowBalanceMicros)
	return err
}

// RecordBillingTransaction appends a ledger entry and moves the balance.
//
// Nothing calls this with a charge today. It exists so that when payments are
// switched on, settlement writes through one function whose accounting is
// visible in one place rather than scattered across handlers.
func (s *Store) RecordBillingTransaction(ctx context.Context, orgID uuid.UUID, kind string, amountMicros int64, txHash, memo string, chainID *int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing_transactions (org_id, kind, amount_micros, tx_hash, chain_id, memo)
		VALUES ($1, $2, $3, NULLIF($4,''), $5, $6)`,
		orgID, kind, amountMicros, txHash, chainID, memo); err != nil {
		return err
	}

	// Top-ups and credits add; charges and refunds subtract from the balance.
	delta := amountMicros
	if kind == "charge" {
		delta = -amountMicros
	}
	if _, err := tx.Exec(ctx, `
		UPDATE billing_accounts SET balance_micros = balance_micros + $2, updated_at = now()
		WHERE org_id = $1`, orgID, delta); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Invoices lists an org's billing periods.
func (s *Store) Invoices(ctx context.Context, orgID uuid.UUID) ([]structs.Invoice, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, period_start::text, period_end::text, active_wallets,
		       rate_micros_per_maw, amount_micros, status, issued_at, paid_at
		FROM invoices WHERE org_id=$1 ORDER BY period_start DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []structs.Invoice{}
	for rows.Next() {
		var i structs.Invoice
		if err := rows.Scan(&i.ID, &i.PeriodStart, &i.PeriodEnd, &i.ActiveWallets,
			&i.RateMicrosPerMAW, &i.AmountMicros, &i.Status, &i.IssuedAt, &i.PaidAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DraftInvoice computes (or refreshes) a draft invoice for a period from
// metered usage. It never issues or charges — that is a deliberate boundary
// until payments are switched on.
func (s *Store) DraftInvoice(ctx context.Context, orgID uuid.UUID, periodStart, periodEnd time.Time) (*structs.Invoice, error) {
	maw, err := s.OrgMonthlyActiveWallets(ctx, orgID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	var rate int64
	if err := s.pool.QueryRow(ctx,
		`SELECT rate_micros_per_maw FROM billing_accounts WHERE org_id=$1`, orgID).Scan(&rate); err != nil {
		return nil, wrap(err)
	}

	var i structs.Invoice
	err = s.pool.QueryRow(ctx, `
		INSERT INTO invoices (org_id, period_start, period_end, active_wallets, rate_micros_per_maw, amount_micros)
		VALUES ($1, $2::date, $3::date, $4, $5, $6)
		ON CONFLICT (org_id, period_start) DO UPDATE SET
			period_end          = EXCLUDED.period_end,
			active_wallets      = EXCLUDED.active_wallets,
			rate_micros_per_maw = EXCLUDED.rate_micros_per_maw,
			amount_micros       = EXCLUDED.amount_micros
		WHERE invoices.status = 'draft'
		RETURNING id, period_start::text, period_end::text, active_wallets,
		          rate_micros_per_maw, amount_micros, status, issued_at, paid_at`,
		orgID, periodStart, periodEnd, maw, rate, int64(maw)*rate,
	).Scan(&i.ID, &i.PeriodStart, &i.PeriodEnd, &i.ActiveWallets,
		&i.RateMicrosPerMAW, &i.AmountMicros, &i.Status, &i.IssuedAt, &i.PaidAt)
	if err != nil {
		return nil, wrap(err)
	}
	return &i, nil
}
