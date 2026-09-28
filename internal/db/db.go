package db

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func InitDB(dbPath string) *DB {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open sqlite db: %v", err)
	}

	// Enable WAL mode for better concurrency
	_, err = db.Exec(`PRAGMA journal_mode=WAL;`)
	if err != nil {
		log.Fatalf("Failed to enable WAL mode: %v", err)
	}

	createSchema(db)

	return &DB{db}
}

func createSchema(db *sql.DB) {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		submissions_close DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS tracks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS judges (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS judge_tracks (
		judge_id TEXT,
		track_id TEXT,
		PRIMARY KEY (judge_id, track_id)
	);

	CREATE TABLE IF NOT EXISTS teams (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS team_members (
		team_id TEXT,
		email TEXT,
		PRIMARY KEY (team_id, email)
	);

	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		team_id TEXT NOT NULL,
		track_id TEXT NOT NULL,
		title TEXT NOT NULL,
		summary TEXT,
		repo_url TEXT,
		submitted_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS scores (
		judge_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		criteria JSON NOT NULL,
		comment TEXT,
		PRIMARY KEY (judge_id, project_id)
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL,
		ref_id TEXT
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		expires_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS pairwise_comparisons (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		judge_id TEXT NOT NULL,
		winner_id TEXT NOT NULL,
		loser_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(schema)
	if err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}
}
