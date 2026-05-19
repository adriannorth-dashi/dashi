-- Dashi database schema (reference copy)
-- The Go service runs these migrations automatically on startup via db.Migrate().
-- This file is provided for documentation and manual inspection only.

CREATE TABLE IF NOT EXISTS customers (
    id         BIGSERIAL    PRIMARY KEY,
    name       TEXT         NOT NULL,
    email      TEXT         NOT NULL,
    api_key    TEXT         NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE
);

-- Phase 2: add rate_limit_rps INT, monthly_quota BIGINT columns here.

CREATE TABLE IF NOT EXISTS sponsorships (
    id             BIGSERIAL    PRIMARY KEY,
    sponsorship_id TEXT         NOT NULL UNIQUE,   -- gas-pool reservation ID (string)
    customer_id    BIGINT       REFERENCES customers(id),
    sender         TEXT         NOT NULL,           -- Sui address of the end-user
    digest         TEXT,                            -- Sui TX digest (filled after confirmation)
    status         TEXT         NOT NULL DEFAULT 'pending',  -- pending | success | failed
    network_fee    BIGINT       NOT NULL,           -- in MIST (1 SUI = 1_000_000_000 MIST)
    service_fee    BIGINT       NOT NULL,           -- in MIST
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at   TIMESTAMPTZ                      -- set when status changes from pending
);

CREATE INDEX IF NOT EXISTS idx_sponsorships_customer_id ON sponsorships(customer_id);
CREATE INDEX IF NOT EXISTS idx_sponsorships_status      ON sponsorships(status);
CREATE INDEX IF NOT EXISTS idx_sponsorships_created_at  ON sponsorships(created_at);
CREATE INDEX IF NOT EXISTS idx_sponsorships_digest      ON sponsorships(digest);
