CREATE TABLE IF NOT EXISTS daily_cashback_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    min_amount NUMERIC(18, 8) NOT NULL DEFAULT 0,
    max_amount NUMERIC(18, 8),
    rate_percent NUMERIC(8, 4) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT daily_cashback_rules_amount_check CHECK (min_amount >= 0 AND (max_amount IS NULL OR max_amount > min_amount)),
    CONSTRAINT daily_cashback_rules_rate_check CHECK (rate_percent > 0 AND rate_percent <= 100)
);

CREATE INDEX IF NOT EXISTS idx_daily_cashback_rules_enabled_order
    ON daily_cashback_rules (enabled, sort_order, min_amount);

CREATE TABLE IF NOT EXISTS daily_cashback_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id BIGINT REFERENCES daily_cashback_rules(id) ON DELETE SET NULL,
    business_date DATE NOT NULL,
    spend_amount NUMERIC(18, 8) NOT NULL,
    rate_percent NUMERIC(8, 4) NOT NULL,
    cashback_amount NUMERIC(18, 8) NOT NULL,
    balance_after NUMERIC(18, 8),
    status VARCHAR(20) NOT NULL DEFAULT 'applied',
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT daily_cashback_records_amount_check CHECK (spend_amount > 0 AND cashback_amount > 0),
    CONSTRAINT daily_cashback_records_status_check CHECK (status IN ('applied'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_cashback_records_user_date
    ON daily_cashback_records (user_id, business_date);

CREATE INDEX IF NOT EXISTS idx_daily_cashback_records_date
    ON daily_cashback_records (business_date DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_daily_cashback_records_user
    ON daily_cashback_records (user_id, business_date DESC);
