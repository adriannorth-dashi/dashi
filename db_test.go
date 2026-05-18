package main

import (
	"context"
	"os"
	"testing"
)

// testPostgresDSN returns the Postgres DSN to use for DB tests.
// Falls back to the standard docker-compose DSN if DATABASE_URL is not set.
func testPostgresDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://gasstation:secret@127.0.0.1:5432/gasstation"
}

// openTestDB connects to a real Postgres instance.
// The test is skipped if Postgres is not reachable — DB tests are optional
// for environments that only have Go available (no docker-compose running).
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(testPostgresDSN())
	if err != nil {
		t.Skipf("Postgres not reachable, skipping DB test: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNewDB_ValidDSN(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestNewDB_EmptyDSN_ReturnsError(t *testing.T) {
	_, err := NewDB("")
	if err == nil {
		t.Fatal("expected error for empty DSN, got nil")
	}
}

func TestMigrate_IdempotentOnRepeat(t *testing.T) {
	db := openTestDB(t)

	// Running Migrate twice must not error (all statements use IF NOT EXISTS).
	if err := db.Migrate(); err != nil {
		t.Fatalf("second Migrate call failed: %v", err)
	}
}

func TestLogSponsorship_InsertsRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	rec := &SponsorshipRecord{
		SponsorshipID: "test-sp-" + t.Name(),
		Sender:        "0x" + "5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6",
		Status:        "pending",
		NetworkFee:    3_000_000,
		ServiceFee:    1_000_000,
	}

	if err := db.LogSponsorship(ctx, rec); err != nil {
		t.Fatalf("LogSponsorship: %v", err)
	}

	// Cleanup so the unique constraint doesn't block future runs.
	t.Cleanup(func() {
		db.conn.ExecContext(ctx, "DELETE FROM sponsorships WHERE sponsorship_id = $1", rec.SponsorshipID)
	})
}

func TestLogSponsorship_DuplicateID_ReturnsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id := "test-dup-" + t.Name()
	rec := &SponsorshipRecord{
		SponsorshipID: id,
		Sender:        "0x" + "5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6",
		Status:        "pending",
		NetworkFee:    3_000_000,
		ServiceFee:    1_000_000,
	}

	t.Cleanup(func() {
		db.conn.ExecContext(ctx, "DELETE FROM sponsorships WHERE sponsorship_id = $1", id)
	})

	if err := db.LogSponsorship(ctx, rec); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.LogSponsorship(ctx, rec); err == nil {
		t.Fatal("expected error on duplicate sponsorship_id, got nil")
	}
}

func TestGetSponsorshipByDigest_NotFound_ReturnsNil(t *testing.T) {
	db := openTestDB(t)

	rec, err := db.GetSponsorshipByDigest(context.Background(), "nonexistent-digest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil for missing digest, got %+v", rec)
	}
}

func TestUpdateSponsorshipStatus_UpdatesRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id := "test-upd-" + t.Name()
	rec := &SponsorshipRecord{
		SponsorshipID: id,
		Sender:        "0x" + "5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6",
		Status:        "pending",
		NetworkFee:    3_000_000,
		ServiceFee:    1_000_000,
	}

	t.Cleanup(func() {
		db.conn.ExecContext(ctx, "DELETE FROM sponsorships WHERE sponsorship_id = $1", id)
	})

	if err := db.LogSponsorship(ctx, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := db.UpdateSponsorshipStatus(ctx, id, "success", "testdigest123"); err != nil {
		t.Fatalf("UpdateSponsorshipStatus: %v", err)
	}
}

func TestGetCustomerByAPIKey_NotFound_ReturnsNil(t *testing.T) {
	db := openTestDB(t)

	c, err := db.GetCustomerByAPIKey(context.Background(), "nonexistent-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil for missing key, got %+v", c)
	}
}

func TestGetSponsorshipByDigest_Found_ReturnsRecord(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	id := "test-get-dig-" + t.Name()
	digest := "testdigest-" + t.Name()

	t.Cleanup(func() {
		db.conn.ExecContext(ctx, "DELETE FROM sponsorships WHERE sponsorship_id = $1", id)
	})

	rec := &SponsorshipRecord{
		SponsorshipID: id,
		Sender:        "0x" + "5757176f7fd65aa19893ec3dd368d88e25e032956af29843bdcbb03ca60f86f6",
		Status:        "pending",
		NetworkFee:    3_000_000,
		ServiceFee:    1_000_000,
	}
	if err := db.LogSponsorship(ctx, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := db.UpdateSponsorshipStatus(ctx, id, "success", digest); err != nil {
		t.Fatalf("update: %v", err)
	}

	found, err := db.GetSponsorshipByDigest(ctx, digest)
	if err != nil {
		t.Fatalf("GetSponsorshipByDigest: %v", err)
	}
	if found == nil {
		t.Fatal("expected record, got nil")
	}
	if found.SponsorshipID != id {
		t.Errorf("SponsorshipID = %q, want %q", found.SponsorshipID, id)
	}
}

func TestGetCustomerByAPIKey_Found_ReturnsCustomer(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	key := "test-customer-key-" + t.Name()
	t.Cleanup(func() {
		db.conn.ExecContext(ctx, "DELETE FROM customers WHERE api_key = $1", key)
	})

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO customers (name, email, api_key) VALUES ($1, $2, $3)`,
		"Test Customer", "test@example.com", key,
	)
	if err != nil {
		t.Fatalf("insert customer: %v", err)
	}

	c, err := db.GetCustomerByAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("GetCustomerByAPIKey: %v", err)
	}
	if c == nil {
		t.Fatal("expected customer, got nil")
	}
	if c.APIKey != key {
		t.Errorf("APIKey = %q, want %q", c.APIKey, key)
	}
}
