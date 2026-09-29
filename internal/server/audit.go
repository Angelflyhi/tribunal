package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

func (s *Server) handleAuditGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	rows, err := s.db.Query(`
		SELECT id, event_id, actor_id, action, subject_type, subject_id, payload_json, created_at, prev_hash, hash
		FROM audit_log
		ORDER BY id ASC
	`)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AuditLog struct {
		ID          string `json:"id"`
		EventID     string `json:"event_id"`
		ActorID     string `json:"actor_id"`
		Action      string `json:"action"`
		SubjectType string `json:"subject_type"`
		SubjectID   string `json:"subject_id"`
		PayloadJSON string `json:"payload_json"`
		CreatedAt   string `json:"created_at"`
		PrevHash    string `json:"prev_hash"`
		Hash        string `json:"hash"`
		Valid       bool   `json:"valid"`
	}

	var logs []AuditLog
	expectedPrevHash := "0000000000000000000000000000000000000000000000000000000000000000"

	for rows.Next() {
		var l AuditLog
		rows.Scan(&l.ID, &l.EventID, &l.ActorID, &l.Action, &l.SubjectType, &l.SubjectID, &l.PayloadJSON, &l.CreatedAt, &l.PrevHash, &l.Hash)

		// Verify hash chain
		msg := l.PrevHash + l.EventID + l.ActorID + l.Action + l.PayloadJSON + l.CreatedAt
		h := sha256.New()
		h.Write([]byte(msg))
		calculatedHash := hex.EncodeToString(h.Sum(nil))

		l.Valid = (calculatedHash == l.Hash) && (l.PrevHash == expectedPrevHash)
		expectedPrevHash = l.Hash

		logs = append(logs, l)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"audit_logs": logs})
}
