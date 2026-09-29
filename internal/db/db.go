package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"time"
	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	Path string
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

	return &DB{DB: db, Path: dbPath}
}

func createSchema(db *sql.DB) {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		phase TEXT DEFAULT 'DRAFT',
		submissions_close DATETIME NOT NULL,
		voting_mode TEXT DEFAULT 'authenticated',
		voting_close DATETIME
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
		tagline TEXT,
		summary TEXT,
		long_description TEXT,
		thumbnail TEXT,
		image_gallery TEXT,
		demo_url TEXT,
		repo_url TEXT,
		live_link TEXT,
		tech_tags TEXT,
		status TEXT DEFAULT 'draft',
		submitted_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS rubric_versions (
		id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		version_number INTEGER NOT NULL,
		criteria_json TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		published BOOLEAN DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS scores (
		judge_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		rubric_version_id TEXT,
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

	CREATE TABLE IF NOT EXISTS public_votes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id TEXT NOT NULL,
		identity TEXT NOT NULL,
		ip_address TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(project_id, identity)
	);

	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id TEXT NOT NULL,
		author_identity TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS judge_assignments (
		id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		judge_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		assigned_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		status TEXT NOT NULL,
		assignment_batch TEXT,
		assignment_reason TEXT,
		UNIQUE(judge_id, project_id)
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		actor_id TEXT NOT NULL,
		action TEXT NOT NULL,
		subject_type TEXT,
		subject_id TEXT,
		payload_json TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		prev_hash TEXT NOT NULL,
		hash TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS webhooks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_type TEXT NOT NULL,
		target_url TEXT NOT NULL,
		secret TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS judge_metrics (
		judge_id TEXT PRIMARY KEY,
		severity FLOAT NOT NULL,
		discrimination FLOAT NOT NULL,
		anomaly_flag TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS certificates (
		id TEXT PRIMARY KEY,
		subject_id TEXT NOT NULL,
		subject_type TEXT NOT NULL,
		event_id TEXT NOT NULL,
		issued_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		signature_hex TEXT NOT NULL
	);
	`

	_, err := db.Exec(schema)
	if err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	// Migrations (ignore errors if columns already exist)
	db.Exec("ALTER TABLE events ADD COLUMN phase TEXT DEFAULT 'DRAFT'")
	db.Exec("ALTER TABLE scores ADD COLUMN rubric_version_id TEXT")
	
	// Project rich fields
	db.Exec("ALTER TABLE projects ADD COLUMN tagline TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN long_description TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN thumbnail TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN image_gallery TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN demo_url TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN live_link TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN tech_tags TEXT")
	db.Exec("ALTER TABLE projects ADD COLUMN status TEXT DEFAULT 'draft'")
}

// LogAudit appends a new cryptographically chained audit record.
func (d *DB) LogAudit(eventID, actorID, action, subjectType, subjectID string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		payloadBytes = []byte("{}")
	}
	payloadJSON := string(payloadBytes)

	// Fetch previous hash
	var prevHash string
	err = d.QueryRow("SELECT hash FROM audit_log WHERE event_id = ? ORDER BY rowid DESC LIMIT 1", eventID).Scan(&prevHash)
	if err == sql.ErrNoRows {
		prevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	} else if err != nil {
		return err
	}

	id := uuid.New().String()
	createdAt := time.Now().UTC().Format(time.RFC3339)

	// hash = SHA256(prev_hash + event_id + actor_id + action + canonical_payload + created_at)
	msg := prevHash + eventID + actorID + action + payloadJSON + createdAt
	h := sha256.New()
	h.Write([]byte(msg))
	hash := hex.EncodeToString(h.Sum(nil))

	_, err = d.Exec(`
		INSERT INTO audit_log (id, event_id, actor_id, action, subject_type, subject_id, payload_json, created_at, prev_hash, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, eventID, actorID, action, subjectType, subjectID, payloadJSON, createdAt, prevHash, hash)

	return err
}

