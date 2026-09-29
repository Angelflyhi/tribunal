package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hackathon-raptors/tribunal/internal/db"
	"github.com/google/uuid"
)

func setupTestDB(t *testing.T) *db.DB {
	database := db.InitDB(":memory:")
	
	// Seed minimal required data
	eventID := uuid.New().String()
	database.Exec("INSERT INTO events (id, name, submissions_close, voting_mode) VALUES (?, 'Test Event', ?, 'authenticated')", 
		eventID, time.Now().Add(1*time.Hour).Format(time.RFC3339))
	
	return database
}

func TestRBACMatrix(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	srv := NewServer(database)
	
	// Create test users
	orgEmail := "org@test.com"
	partEmail := "part@test.com"
	
	database.Exec("INSERT INTO users (id, email, password_hash, role) VALUES (?, ?, ?, ?)", uuid.New().String(), orgEmail, "hash", "organizer")
	database.Exec("INSERT INTO users (id, email, password_hash, role) VALUES (?, ?, ?, ?)", uuid.New().String(), partEmail, "hash", "participant")
	
	orgToken := "org-token"
	partToken := "part-token"
	database.Exec("INSERT INTO sessions (token, user_id, expires_at) SELECT ?, id, ? FROM users WHERE email = ?", orgToken, time.Now().Add(1*time.Hour).Format(time.RFC3339), orgEmail)
	database.Exec("INSERT INTO sessions (token, user_id, expires_at) SELECT ?, id, ? FROM users WHERE email = ?", partToken, time.Now().Add(1*time.Hour).Format(time.RFC3339), partEmail)

	tests := []struct {
		name       string
		method     string
		url        string
		token      string
		wantStatus int
	}{
		// Participant access
		{"Part -> Dashboard", "GET", "/dashboard", partToken, http.StatusOK},
		{"Part -> Export CSV (Deny)", "GET", "/api/export.csv", partToken, http.StatusForbidden},
		{"Part -> Assignments (Deny)", "GET", "/api/assignments", partToken, http.StatusForbidden},
		
		// Organizer access
		{"Org -> Export CSV", "GET", "/api/export.csv", orgToken, http.StatusOK},
		
		// Unauthenticated access
		{"Anon -> Dashboard (Deny)", "GET", "/dashboard", "", http.StatusSeeOther}, // redirects to login
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			if tt.token != "" {
				req.AddCookie(&http.Cookie{Name: "session", Value: tt.token})
			}
			w := httptest.NewRecorder()
			
			srv.Router().ServeHTTP(w, req)
			
			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuditLogChain(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()
	
	err := database.LogAudit("event1", "admin1", "test_action_1", "system", "123", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("LogAudit failed: %v", err)
	}
	
	err = database.LogAudit("event1", "admin1", "test_action_2", "system", "456", map[string]string{"foo": "baz"})
	if err != nil {
		t.Fatalf("LogAudit 2 failed: %v", err)
	}
	
	rows, err := database.Query("SELECT id, prev_hash, hash FROM audit_log ORDER BY id ASC")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()
	
	var records []struct{id, prev, hash string}
	for rows.Next() {
		var id, p, h string
		rows.Scan(&id, &p, &h)
		records = append(records, struct{id, prev, hash string}{id, p, h})
	}
	
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}
	
	if records[0].prev != "0000000000000000000000000000000000000000000000000000000000000000" {
		t.Errorf("First record prev_hash incorrect")
	}
	
	if records[1].prev != records[0].hash {
		t.Errorf("Cryptographic chain broken! record[1].prev %s != record[0].hash %s", records[1].prev, records[0].hash)
	}
}
