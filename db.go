// Copyright (C) 2025 Dashi Project
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps a sql.DB and exposes all Dashi-specific queries.
// All SQL is in this file — handlers never construct queries directly.
type DB struct {
	conn *sql.DB
}

// SponsorshipRecord maps to a row in the sponsorships table.
type SponsorshipRecord struct {
	SponsorshipID string
	CustomerID    *int64
	Sender        string
	Digest        string
	Status        string
	NetworkFee    int64 // in MIST (1 SUI = 1_000_000_000 MIST)
	ServiceFee    int64 // in MIST
}

// Customer maps to a row in the customers table.
type Customer struct {
	ID       int64
	Name     string
	Email    string
	APIKey   string
	IsActive bool
}

// NewDB opens a connection pool to Postgres and verifies connectivity.
func NewDB(dsn string) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)
	conn.SetConnMaxIdleTime(2 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close releases all database connections.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Migrate runs the schema creation SQL. Safe to call on every startup
// because all statements use IF NOT EXISTS.
func (d *DB) Migrate() error {
	_, err := d.conn.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS customers (
    id         BIGSERIAL    PRIMARY KEY,
    name       TEXT         NOT NULL,
    email      TEXT         NOT NULL,
    api_key    TEXT         NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS sponsorships (
    id             BIGSERIAL    PRIMARY KEY,
    sponsorship_id TEXT         NOT NULL UNIQUE,
    customer_id    BIGINT       REFERENCES customers(id),
    sender         TEXT         NOT NULL,
    digest         TEXT,
    status         TEXT         NOT NULL DEFAULT 'pending',
    network_fee    BIGINT       NOT NULL,
    service_fee    BIGINT       NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sponsorships_customer_id ON sponsorships(customer_id);
CREATE INDEX IF NOT EXISTS idx_sponsorships_status      ON sponsorships(status);
CREATE INDEX IF NOT EXISTS idx_sponsorships_created_at  ON sponsorships(created_at);
CREATE INDEX IF NOT EXISTS idx_sponsorships_digest      ON sponsorships(digest);
`

// LogSponsorship inserts a new sponsorship record. ON CONFLICT resets status so that
// repeated test runs with the same reservation ID (gas pool resets its counter on restart)
// don't leave a stale "failed" record blocking a fresh attempt.
func (d *DB) LogSponsorship(ctx context.Context, rec *SponsorshipRecord) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO sponsorships (sponsorship_id, customer_id, sender, status, network_fee, service_fee)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (sponsorship_id) DO UPDATE
		  SET sender      = EXCLUDED.sender,
		      status      = EXCLUDED.status,
		      network_fee = EXCLUDED.network_fee,
		      service_fee = EXCLUDED.service_fee,
		      digest      = NULL,
		      completed_at = NULL
	`, rec.SponsorshipID, rec.CustomerID, rec.Sender, rec.Status, rec.NetworkFee, rec.ServiceFee)
	return err
}

// UpdateSponsorshipStatus sets status and digest on a completed sponsorship.
func (d *DB) UpdateSponsorshipStatus(ctx context.Context, sponsorshipID, status, digest string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE sponsorships
		SET status = $1, digest = $2, completed_at = NOW()
		WHERE sponsorship_id = $3
	`, status, digest, sponsorshipID)
	return err
}

// GetSponsorshipByDigest looks up a sponsorship by its Sui transaction digest.
// Returns nil, nil when no row is found.
func (d *DB) GetSponsorshipByDigest(ctx context.Context, digest string) (*SponsorshipRecord, error) {
	var rec SponsorshipRecord
	err := d.conn.QueryRowContext(ctx, `
		SELECT sponsorship_id, sender, status, network_fee, service_fee
		FROM sponsorships
		WHERE digest = $1
	`, digest).Scan(&rec.SponsorshipID, &rec.Sender, &rec.Status, &rec.NetworkFee, &rec.ServiceFee)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// GetSponsorshipByID looks up a sponsorship by its numeric reservation ID.
// Returns nil, nil when no row is found.
func (d *DB) GetSponsorshipByID(ctx context.Context, sponsorshipID string) (*SponsorshipRecord, error) {
	var rec SponsorshipRecord
	var digest sql.NullString
	err := d.conn.QueryRowContext(ctx, `
		SELECT sponsorship_id, sender, COALESCE(digest, ''), status, network_fee, service_fee
		FROM sponsorships
		WHERE sponsorship_id = $1
	`, sponsorshipID).Scan(&rec.SponsorshipID, &rec.Sender, &digest, &rec.Status, &rec.NetworkFee, &rec.ServiceFee)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.Digest = digest.String
	return &rec, nil
}

// GetCustomerByAPIKey looks up an active customer by API key.
// Returns nil, nil when no row is found.
func (d *DB) GetCustomerByAPIKey(ctx context.Context, apiKey string) (*Customer, error) {
	var c Customer
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, name, email, api_key, is_active
		FROM customers
		WHERE api_key = $1 AND is_active = TRUE
	`, apiKey).Scan(&c.ID, &c.Name, &c.Email, &c.APIKey, &c.IsActive)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
