package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "path to the Gameplay SQLite database")
	owner := flag.String("owner", "", "retired Peer public key")
	petID := flag.String("pet", "", "deleted Pet ID")
	flag.Parse()
	if *dbPath == "" || *owner == "" || *petID == "" {
		fmt.Fprintln(os.Stderr, "db, owner, and pet are required")
		os.Exit(2)
	}

	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		fail("open Gameplay database", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var rows int
	err = db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gameplay_pet_adoption_reservations WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pet_drive_ticks WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_points_accounts WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_points_transactions WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_badges WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_game_results WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_reward_grants WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_drive_fact_outbox WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE beneficiary_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pets WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pending_deletions WHERE owner_public_key = ?) +
		(SELECT COUNT(*) FROM gameplay_pending_deletion_locators WHERE owner_public_key = ?)`,
		*owner, *owner, *owner, *owner, *owner, *owner, *owner,
		*owner, *owner, *owner, *owner, *owner, *owner).Scan(&rows)
	if err != nil {
		fail("count Peer-owned Gameplay rows", err)
	}
	if rows != 0 {
		fmt.Fprintf(os.Stderr, "retired Peer still owns %d Gameplay rows\n", rows)
		os.Exit(1)
	}

	var pets int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_pets WHERE owner_public_key = ? AND id = ?`, *owner, *petID).Scan(&pets); err != nil {
		fail("count deleted Pet", err)
	}
	if pets != 0 {
		fmt.Fprintln(os.Stderr, "deleted Pet row remains")
		os.Exit(1)
	}
}

func fail(operation string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", operation, err)
	os.Exit(1)
}
