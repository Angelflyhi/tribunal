package server

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
)

// GenerateAssignments performs the algorithmic judge assignment.
func (s *Server) GenerateAssignments(eventID string, targetPerProject int, seed int64) error {
	// Fetch tracks
	rows, err := s.db.Query("SELECT id FROM tracks")
	if err != nil {
		return err
	}
	defer rows.Close()
	var tracks []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		tracks = append(tracks, id)
	}

	// Fetch projects
	pRows, err := s.db.Query("SELECT id, track_id, team_id FROM projects")
	if err != nil {
		return err
	}
	defer pRows.Close()
	
	type ProjectData struct {
		ID      string
		TrackID string
		TeamID  string
	}
	var projects []ProjectData
	for pRows.Next() {
		var p ProjectData
		pRows.Scan(&p.ID, &p.TrackID, &p.TeamID)
		projects = append(projects, p)
	}

	// Fetch judges and their tracks
	jRows, err := s.db.Query(`
		SELECT j.id, u.email, jt.track_id 
		FROM users u 
		JOIN judges j ON u.ref_id = j.id
		JOIN judge_tracks jt ON j.id = jt.judge_id
		WHERE u.role = 'judge'
	`)
	if err != nil {
		return err
	}
	defer jRows.Close()

	type JudgeData struct {
		ID     string
		Email  string
		Tracks map[string]bool
	}
	
	judgeMap := make(map[string]*JudgeData)
	var judgeIDs []string
	for jRows.Next() {
		var id, email, trackID string
		jRows.Scan(&id, &email, &trackID)
		if _, ok := judgeMap[id]; !ok {
			judgeMap[id] = &JudgeData{ID: id, Email: email, Tracks: make(map[string]bool)}
			judgeIDs = append(judgeIDs, id)
		}
		judgeMap[id].Tracks[trackID] = true
	}

	// Sort IDs for determinism
	sort.Strings(judgeIDs)
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ID < projects[j].ID
	})

	// Fetch team members to avoid conflicts
	tmRows, err := s.db.Query("SELECT team_id, email FROM team_members")
	if err != nil {
		return err
	}
	defer tmRows.Close()
	
	teamMembers := make(map[string][]string) // team_id -> emails
	for tmRows.Next() {
		var tid, email string
		tmRows.Scan(&tid, &email)
		teamMembers[tid] = append(teamMembers[tid], email)
	}

	// Deterministic random
	rng := rand.New(rand.NewSource(seed))

	var assignments []struct {
		JudgeID   string
		ProjectID string
	}

	judgeLoads := make(map[string]int)

	// Assign judges to projects
	for _, p := range projects {
		var eligible []string
		for _, jID := range judgeIDs {
			j := judgeMap[jID]
			// Constraint 1: Judge eligible for project track
			if !j.Tracks[p.TrackID] {
				continue
			}
			// Constraint 2 & 3: Judge not on project team / no conflict
			conflict := false
			for _, memberEmail := range teamMembers[p.TeamID] {
				if memberEmail == j.Email {
					conflict = true
					break
				}
			}
			if !conflict {
				eligible = append(eligible, jID)
			}
		}

		// Shuffle eligible deterministically
		rng.Shuffle(len(eligible), func(i, j int) {
			eligible[i], eligible[j] = eligible[j], eligible[i]
		})

		// Sort by workload (Load balancing)
		sort.SliceStable(eligible, func(i, j int) bool {
			return judgeLoads[eligible[i]] < judgeLoads[eligible[j]]
		})

		// Pick top N
		limit := targetPerProject
		if limit > len(eligible) {
			limit = len(eligible)
		}

		for i := 0; i < limit; i++ {
			jID := eligible[i]
			assignments = append(assignments, struct{ JudgeID, ProjectID string }{jID, p.ID})
			judgeLoads[jID]++
		}
	}

	// Write assignments
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	
	batchID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	stmt, err := tx.Prepare(`
		INSERT INTO judge_assignments (id, event_id, judge_id, project_id, assigned_at, status, assignment_batch)
		VALUES (?, ?, ?, ?, ?, 'assigned', ?)
		ON CONFLICT(judge_id, project_id) DO NOTHING
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, a := range assignments {
		id := uuid.New().String()
		_, err = stmt.Exec(id, eventID, a.JudgeID, a.ProjectID, now, batchID)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Audit Log
	s.db.LogAudit(eventID, "system", "ASSIGNMENT_BATCH_GENERATED", "batch", batchID, map[string]interface{}{
		"target_per_project": targetPerProject,
		"seed":               seed,
		"assignments_count":  len(assignments),
	})

	return nil
}

func (s *Server) handleAssignmentsPost(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var eventID string
	s.db.QueryRow("SELECT id FROM events LIMIT 1").Scan(&eventID)

	type Request struct {
		Target int   `json:"target"`
		Seed   int64 `json:"seed"`
	}
	var req Request
	json.NewDecoder(r.Body).Decode(&req)
	
	if req.Target <= 0 {
		req.Target = 3
	}
	if req.Seed == 0 {
		req.Seed = time.Now().UnixNano()
	}

	err := s.GenerateAssignments(eventID, req.Target, req.Seed)
	if err != nil {
		http.Error(w, "Assignment Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleJudgingProgressGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Fetch judging stats
	rows, err := s.db.Query(`
		SELECT j.name, count(a.id) as assigned, sum(case when a.status = 'completed' then 1 else 0 end) as completed
		FROM judge_assignments a
		JOIN judges j ON a.judge_id = j.id
		GROUP BY a.judge_id
	`)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type JudgeProgress struct {
		Name      string `json:"name"`
		Assigned  int    `json:"assigned"`
		Completed int    `json:"completed"`
		Progress  int    `json:"progress"`
	}

	var progress []JudgeProgress
	for rows.Next() {
		var p JudgeProgress
		rows.Scan(&p.Name, &p.Assigned, &p.Completed)
		if p.Assigned > 0 {
			p.Progress = int(float64(p.Completed) / float64(p.Assigned) * 100)
		}
		progress = append(progress, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"judges": progress,
	})
}
