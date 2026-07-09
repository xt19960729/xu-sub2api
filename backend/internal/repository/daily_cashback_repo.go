package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type dailyCashbackRepository struct {
	client *dbent.Client
	sql    *sql.DB
}

func NewDailyCashbackRepository(client *dbent.Client, sqlDB *sql.DB) service.DailyCashbackRepository {
	return &dailyCashbackRepository{client: client, sql: sqlDB}
}

func (r *dailyCashbackRepository) ListRules(ctx context.Context) ([]service.DailyCashbackRule, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT id,
       name,
       enabled,
       min_amount::double precision,
       max_amount::double precision,
       rate_percent::double precision,
       sort_order,
       created_at,
       updated_at
FROM daily_cashback_rules
ORDER BY sort_order ASC, min_amount ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	rules := make([]service.DailyCashbackRule, 0)
	for rows.Next() {
		var rule service.DailyCashbackRule
		var maxAmount sql.NullFloat64
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Enabled,
			&rule.MinAmount,
			&maxAmount,
			&rule.RatePercent,
			&rule.SortOrder,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rule.MaxAmount = nullableFloat64Ptr(maxAmount)
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *dailyCashbackRepository) CreateRule(ctx context.Context, rule service.DailyCashbackRule) (*service.DailyCashbackRule, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
INSERT INTO daily_cashback_rules (name, enabled, min_amount, max_amount, rate_percent, sort_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING id, name, enabled, min_amount::double precision, max_amount::double precision, rate_percent::double precision, sort_order, created_at, updated_at`,
		rule.Name,
		rule.Enabled,
		rule.MinAmount,
		nullableFloat64Arg(rule.MaxAmount),
		rule.RatePercent,
		rule.SortOrder,
	)
	if err != nil {
		return nil, fmt.Errorf("create daily cashback rule: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanDailyCashbackRule(rows)
}

func (r *dailyCashbackRepository) UpdateRule(ctx context.Context, rule service.DailyCashbackRule) (*service.DailyCashbackRule, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
UPDATE daily_cashback_rules
SET name = $2,
    enabled = $3,
    min_amount = $4,
    max_amount = $5,
    rate_percent = $6,
    sort_order = $7,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, enabled, min_amount::double precision, max_amount::double precision, rate_percent::double precision, sort_order, created_at, updated_at`,
		rule.ID,
		rule.Name,
		rule.Enabled,
		rule.MinAmount,
		nullableFloat64Arg(rule.MaxAmount),
		rule.RatePercent,
		rule.SortOrder,
	)
	if err != nil {
		return nil, fmt.Errorf("update daily cashback rule: %w", err)
	}
	defer func() { _ = rows.Close() }()
	updated, err := scanDailyCashbackRule(rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrDailyCashbackRuleNotFound
		}
		return nil, err
	}
	return updated, nil
}

func (r *dailyCashbackRepository) DeleteRule(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	res, err := client.ExecContext(ctx, "DELETE FROM daily_cashback_rules WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete daily cashback rule: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return service.ErrDailyCashbackRuleNotFound
	}
	return nil
}

func (r *dailyCashbackRepository) ListDailySpends(ctx context.Context, startTime, endTime time.Time) ([]service.DailyCashbackSpend, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT user_id,
       COALESCE(SUM(actual_cost), 0)::double precision AS spend_amount
FROM usage_logs
WHERE created_at >= $1
  AND created_at < $2
  AND actual_cost > 0
GROUP BY user_id
HAVING COALESCE(SUM(actual_cost), 0) > 0
ORDER BY user_id ASC`, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("list daily cashback spends: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.DailyCashbackSpend, 0)
	for rows.Next() {
		var item service.DailyCashbackSpend
		if err := rows.Scan(&item.UserID, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *dailyCashbackRepository) InsertRecordAndCredit(ctx context.Context, record service.DailyCashbackRecord) (bool, *service.DailyCashbackRecord, error) {
	var applied bool
	var out *service.DailyCashbackRecord
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		rows, err := txClient.QueryContext(txCtx, `
INSERT INTO daily_cashback_records (
    user_id,
    rule_id,
    business_date,
    spend_amount,
    rate_percent,
    cashback_amount,
    status,
    applied_at,
    created_at,
    updated_at
)
VALUES ($1, $2, $3::date, $4, $5, $6, 'applied', NOW(), NOW(), NOW())
ON CONFLICT (user_id, business_date) DO NOTHING
RETURNING id, business_date::text, applied_at`,
			record.UserID,
			nullableInt64Arg(record.RuleID),
			record.BusinessDate,
			record.SpendAmount,
			record.RatePercent,
			record.CashbackAmount,
		)
		if err != nil {
			return fmt.Errorf("insert daily cashback record: %w", err)
		}
		if !rows.Next() {
			if closeErr := rows.Close(); closeErr != nil {
				return closeErr
			}
			applied = false
			return nil
		}

		outRecord := record
		if err := rows.Scan(&outRecord.ID, &outRecord.BusinessDate, &outRecord.AppliedAt); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}

		balanceRows, err := txClient.QueryContext(txCtx, `
UPDATE users
SET balance = balance + $2,
    total_recharged = total_recharged + $2,
    updated_at = NOW()
WHERE id = $1
RETURNING balance::double precision`, record.UserID, record.CashbackAmount)
		if err != nil {
			return fmt.Errorf("credit daily cashback balance: %w", err)
		}
		if !balanceRows.Next() {
			_ = balanceRows.Close()
			return service.ErrUserNotFound
		}
		var balanceAfter float64
		if err := balanceRows.Scan(&balanceAfter); err != nil {
			_ = balanceRows.Close()
			return err
		}
		if err := balanceRows.Close(); err != nil {
			return err
		}
		outRecord.BalanceAfter = &balanceAfter

		if _, err := txClient.ExecContext(txCtx,
			"UPDATE daily_cashback_records SET balance_after = $2, updated_at = NOW() WHERE id = $1",
			outRecord.ID,
			balanceAfter,
		); err != nil {
			return fmt.Errorf("update daily cashback balance snapshot: %w", err)
		}

		applied = true
		out = &outRecord
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return applied, out, nil
}

func (r *dailyCashbackRepository) ListRecords(ctx context.Context, filter service.DailyCashbackRecordFilter) ([]service.DailyCashbackRecord, int64, error) {
	client := clientFromContext(ctx, r.client)
	where, args := buildDailyCashbackRecordWhere(filter)

	total, err := scanInt64(ctx, client, `
SELECT COUNT(*)
FROM daily_cashback_records dcr
JOIN users u ON u.id = dcr.user_id
LEFT JOIN daily_cashback_rules dcrr ON dcrr.id = dcr.rule_id
`+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("count daily cashback records: %w", err)
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)

	rows, err := client.QueryContext(ctx, `
SELECT dcr.id,
       dcr.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       dcr.rule_id,
       COALESCE(dcrr.name, ''),
       dcr.business_date::text,
       dcr.spend_amount::double precision,
       dcr.rate_percent::double precision,
       dcr.cashback_amount::double precision,
       dcr.balance_after::double precision,
       dcr.status,
       dcr.applied_at
FROM daily_cashback_records dcr
JOIN users u ON u.id = dcr.user_id
LEFT JOIN daily_cashback_rules dcrr ON dcrr.id = dcr.rule_id
`+where+`
ORDER BY dcr.business_date DESC, dcr.id DESC
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list daily cashback records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.DailyCashbackRecord, 0)
	for rows.Next() {
		var item service.DailyCashbackRecord
		var ruleID sql.NullInt64
		var balanceAfter sql.NullFloat64
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.UserEmail,
			&item.Username,
			&ruleID,
			&item.RuleName,
			&item.BusinessDate,
			&item.SpendAmount,
			&item.RatePercent,
			&item.CashbackAmount,
			&balanceAfter,
			&item.Status,
			&item.AppliedAt,
		); err != nil {
			return nil, 0, err
		}
		if ruleID.Valid {
			v := ruleID.Int64
			item.RuleID = &v
		}
		item.BalanceAfter = nullableFloat64Ptr(balanceAfter)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func scanDailyCashbackRule(rows *sql.Rows) (*service.DailyCashbackRule, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	var rule service.DailyCashbackRule
	var maxAmount sql.NullFloat64
	if err := rows.Scan(
		&rule.ID,
		&rule.Name,
		&rule.Enabled,
		&rule.MinAmount,
		&maxAmount,
		&rule.RatePercent,
		&rule.SortOrder,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return nil, err
	}
	rule.MaxAmount = nullableFloat64Ptr(maxAmount)
	return &rule, rows.Err()
}

func buildDailyCashbackRecordWhere(filter service.DailyCashbackRecordFilter) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if strings.TrimSpace(filter.BusinessDate) != "" {
		args = append(args, strings.TrimSpace(filter.BusinessDate))
		clauses = append(clauses, fmt.Sprintf("dcr.business_date = $%d::date", len(args)))
	}
	search := strings.TrimSpace(filter.Search)
	if search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		placeholder := fmt.Sprintf("$%d", len(args))
		clauses = append(clauses, "(LOWER(u.email) LIKE "+placeholder+" OR LOWER(u.username) LIKE "+placeholder+" OR u.id::text LIKE "+placeholder+")")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (r *dailyCashbackRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin daily cashback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit daily cashback transaction: %w", err)
	}
	return nil
}

func nullableFloat64Arg(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
