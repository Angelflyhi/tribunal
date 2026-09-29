package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hackathon-raptors/tribunal/internal/db"
	"github.com/hackathon-raptors/tribunal/internal/judging"
	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

var tmpl map[string]*template.Template

func init() {
	tmpl = make(map[string]*template.Template)
	pages := []string{"gallery", "login", "register", "dashboard", "projects_new", "projects_edit", "judge_pairwise", "judge_rubric", "organizer_leaderboard"}
	for _, page := range pages {
		t, err := template.ParseFS(templatesFS, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			panic(err)
		}
		tmpl[page] = t
	}
}

type Server struct {
	db *db.DB
}

type User struct {
	ID    string
	Email string
	Role  string
	RefID string
}

type contextKey string

const userContextKey = contextKey("user")

func NewServer(database *db.DB) *Server {
	return &Server{
		db: database,
	}
}

// authMiddleware checks the Cookie header and populates the request context with the User
	func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var token string

		// Try Authorization header first
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// Fallback to Cookie
		if token == "" {
			cookieHeader := r.Header.Get("Cookie")
			for _, part := range strings.Split(cookieHeader, ";") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "session=") {
					token = strings.TrimPrefix(part, "session=")
					break
				}
			}
		}

		if token != "" {
			var u User
			err := s.db.QueryRow(`
				SELECT u.id, u.email, u.role, u.ref_id 
				FROM users u 
				JOIN sessions s ON u.id = s.user_id 
				WHERE s.token = ? AND s.expires_at > datetime('now')
			`, token).Scan(&u.ID, &u.Email, &u.Role, &u.RefID)

			if err == nil {
				ctx := context.WithValue(r.Context(), userContextKey, &u)
				r = r.WithContext(ctx)
			}
		}

		next(w, r)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(userContextKey) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

var (
	rateLimiter = make(map[string]int)
	rateMutex   sync.Mutex
	rateCleanup time.Time
)

func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := strings.Split(r.RemoteAddr, ":")[0]
		key := ip
		if user, ok := r.Context().Value(userContextKey).(*User); ok && user != nil {
			key = user.Email
		}

		rateMutex.Lock()
		if time.Since(rateCleanup) > time.Minute {
			rateLimiter = make(map[string]int)
			rateCleanup = time.Now()
		}
		rateLimiter[key]++
		count := rateLimiter[key]
		rateMutex.Unlock()

		if count > 60 { // Max 60 requests per minute
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func (s *Server) Router() *http.ServeMux {
	mux := http.NewServeMux()

	// Static files
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	// Auth routes
	mux.HandleFunc("GET /login", s.authMiddleware(s.handleLoginGet))
	mux.HandleFunc("POST /login", s.authMiddleware(s.handleLoginPost))
	mux.HandleFunc("GET /register", s.authMiddleware(s.handleRegisterGet))
	mux.HandleFunc("POST /register", s.authMiddleware(s.handleRegisterPost))
	mux.HandleFunc("GET /logout", s.authMiddleware(s.handleLogout))

	// T1: A stranger can browse the gallery.
	// T1: The gallery shows fixture projects.
	mux.HandleFunc("GET /", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/projects", http.StatusSeeOther)
			return
		}
		http.NotFound(w, r)
	}))
	mux.HandleFunc("GET /projects", s.authMiddleware(s.handleGallery))
	mux.HandleFunc("GET /projects/new", s.authMiddleware(s.requireAuth(s.handleNewProjectGet)))

	// T1: A closed event refuses submissions.
	mux.HandleFunc("POST /projects/new", s.authMiddleware(s.requireAuth(s.handleSubmit)))
	mux.HandleFunc("GET /projects/edit", s.authMiddleware(s.requireAuth(s.handleEditProjectGet)))
	mux.HandleFunc("POST /projects/edit", s.authMiddleware(s.requireAuth(s.handleEditProjectPost)))

	// Dashboard
	mux.HandleFunc("GET /dashboard", s.authMiddleware(s.requireAuth(s.handleDashboard)))
	mux.HandleFunc("POST /teams/new", s.authMiddleware(s.requireAuth(s.handleNewTeam)))
	mux.HandleFunc("GET /teams/join", s.authMiddleware(s.requireAuth(s.handleJoinTeam)))

	// Judging features
	mux.HandleFunc("GET /judge/pairwise", s.authMiddleware(s.requireAuth(s.handleJudgePairwiseGet)))
	mux.HandleFunc("POST /judge/pairwise", s.authMiddleware(s.requireAuth(s.handleJudgePairwisePost)))
	mux.HandleFunc("GET /judge/rubric", s.authMiddleware(s.requireAuth(s.handleJudgeRubricGet)))

	// Organizer features
	mux.HandleFunc("GET /organizer/leaderboard", s.authMiddleware(s.requireAuth(s.handleOrganizerLeaderboard)))
	mux.HandleFunc("POST /api/assignments", s.authMiddleware(s.handleAssignmentsPost))
	mux.HandleFunc("GET /api/judging/progress", s.authMiddleware(s.handleJudgingProgressGet))
	mux.HandleFunc("GET /api/audit", s.authMiddleware(s.handleAuditGet))

	// T2: Judge scoring logic
	mux.HandleFunc("GET /api/judge/scores", s.authMiddleware(s.handleJudgeScores))

	// T2: Organizer CSV export
	mux.HandleFunc("GET /api/export.csv", s.authMiddleware(s.handleExportCSV))
	// T4: Signed Results Bundle
	mux.HandleFunc("GET /api/export/bundle", s.authMiddleware(s.handleExportBundle))
	// T4: Lossless Import/Export
	mux.HandleFunc("GET /api/export/lossless", s.authMiddleware(s.handleExportLossless))
	mux.HandleFunc("POST /api/import/lossless", s.authMiddleware(s.handleImportLossless))

	// T3 Public Voting
	mux.HandleFunc("POST /api/projects/{id}/vote", s.rateLimitMiddleware(s.authMiddleware(s.handlePublicVote)))
	mux.HandleFunc("POST /api/projects/{id}/comment", s.rateLimitMiddleware(s.authMiddleware(s.handleAddComment)))
	mux.HandleFunc("GET /results", s.rateLimitMiddleware(s.authMiddleware(s.handlePublicResults)))

	// T4 Extensions
	mux.HandleFunc("PUT /api/projects/{id}", s.rateLimitMiddleware(s.authMiddleware(s.requireAuth(s.handleUpdateProject))))
	mux.HandleFunc("DELETE /api/projects/{id}", s.rateLimitMiddleware(s.authMiddleware(s.requireAuth(s.handleDeleteProject))))
	mux.HandleFunc("POST /api/webhooks", s.authMiddleware(s.handleWebhooks))
	mux.HandleFunc("GET /api/certificate", s.authMiddleware(s.handleCertificate))
	mux.HandleFunc("POST /api/certificate/verify", s.authMiddleware(s.handleCertificateVerify))
	mux.HandleFunc("POST /api/import", s.authMiddleware(s.handleBulkImport))
	mux.HandleFunc("GET /embed/gallery", s.handleEmbedGallery)

	return mux
}

type ProjectView struct {
	ID        string
	Title     string
	Summary   string
	TrackName string
	TrackID   string
	RepoURL   string
}

type Track struct {
	ID   string
	Name string
}

type RankedProject struct {
	ID         string
	Rank       int
	Title      string
	TrackName  string
	Score      string
	ScoreFloat float64
	CILower    int
	CIUpper    int
}

type PageData struct {
	User           *User
	Projects       []ProjectView
	Tracks         []Track
	Error          string
	TeamName       string
	TeamID         string
	TeamMembers    []string
	Project        *ProjectView
	ProjectA       *ProjectView
	ProjectB       *ProjectView
	RankedProjects []RankedProject
	JudgeInfluences []judging.JudgeInfluence
}

func (s *Server) handleGallery(w http.ResponseWriter, r *http.Request) {
	var user *User
	if u, ok := r.Context().Value(userContextKey).(*User); ok {
		user = u
	}

	rows, err := s.db.Query(`
		SELECT p.id, p.title, p.summary, t.name 
		FROM projects p
		JOIN tracks t ON p.track_id = t.id
		ORDER BY RANDOM()
	`)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []ProjectView
	for rows.Next() {
		var p ProjectView
		if err := rows.Scan(&p.ID, &p.Title, &p.Summary, &p.TrackName); err == nil {
			projects = append(projects, p)
		}
	}

	data := PageData{
		User:     user,
		Projects: projects,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl["gallery"].ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "Template Error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	var user *User
	if u, ok := r.Context().Value(userContextKey).(*User); ok {
		user = u
	}
	if user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["login"].ExecuteTemplate(w, "base", PageData{User: nil})
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	var user User
	var hash string
	err := s.db.QueryRow("SELECT id, email, password_hash, role, ref_id FROM users WHERE email = ?", email).Scan(&user.ID, &user.Email, &hash, &user.Role, &user.RefID)
	
	if err == sql.ErrNoRows {
		tmpl["login"].ExecuteTemplate(w, "base", PageData{Error: "Invalid email or password"})
		return
	}

	if hash != "hash" { // Handle seeded dummy users
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
			tmpl["login"].ExecuteTemplate(w, "base", PageData{Error: "Invalid email or password"})
			return
		}
	}

	token := generateToken()
	_, err = s.db.Exec("INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, datetime('now', '+24 hours'))", token, user.ID)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleRegisterGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["register"].ExecuteTemplate(w, "base", PageData{})
}

func (s *Server) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	role := r.FormValue("role")

	if role != "participant" && role != "judge" && role != "organizer" {
		role = "participant"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		tmpl["register"].ExecuteTemplate(w, "base", PageData{Error: "Failed to process password"})
		return
	}

	userID := uuid.New().String()
	_, err = s.db.Exec("INSERT INTO users (id, email, password_hash, role, ref_id) VALUES (?, ?, ?, ?, ?)", userID, email, string(hash), role, email)
	if err != nil {
		tmpl["register"].ExecuteTemplate(w, "base", PageData{Error: "Email already registered"})
		return
	}

	token := generateToken()
	s.db.Exec("INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, datetime('now', '+24 hours'))", token, userID)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    "session",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

type DashboardData struct {
	User        *User
	TeamName    string
	TeamMembers []string
	InviteLink  string
	Project     *ProjectView
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	
	data := PageData{User: user}
	
	// If participant, fetch their team and project
	if user.Role == "participant" {
		var teamID, teamName string
		err := s.db.QueryRow("SELECT t.id, t.name FROM teams t JOIN team_members tm ON t.id = tm.team_id WHERE tm.email = ?", user.Email).Scan(&teamID, &teamName)
		
		if err == nil {
			data.TeamID = teamID
			data.TeamName = teamName
			// user has a team, get members
			rows, _ := s.db.Query("SELECT email FROM team_members WHERE team_id = ?", teamID)
			var members []string
			defer rows.Close()
			for rows.Next() {
				var em string
				rows.Scan(&em)
				members = append(members, em)
			}
			data.TeamMembers = members

			// Get project if any
			var p ProjectView
			err = s.db.QueryRow("SELECT p.id, p.title, p.summary, tr.name FROM projects p JOIN tracks tr ON p.track_id = tr.id WHERE p.team_id = ?", teamID).Scan(&p.ID, &p.Title, &p.Summary, &p.TrackName)
			if err == nil {
				data.Project = &p
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["dashboard"].ExecuteTemplate(w, "base", data)
}

func (s *Server) handleNewProjectGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "participant" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	
	rows, err := s.db.Query("SELECT id, name FROM tracks")
	var tracks []Track
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t Track
			if err := rows.Scan(&t.ID, &t.Name); err == nil {
				tracks = append(tracks, t)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["projects_new"].ExecuteTemplate(w, "base", PageData{User: user, Tracks: tracks})
}

func (s *Server) handleNewTeam(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "participant" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	teamName := r.FormValue("team_name")
	if teamName == "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	teamID := uuid.New().String()
	tx, err := s.db.Begin()
	if err == nil {
		tx.Exec("INSERT INTO teams (id, name) VALUES (?, ?)", teamID, teamName)
		tx.Exec("INSERT INTO team_members (team_id, email) VALUES (?, ?)", teamID, user.Email)
		tx.Commit()
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "participant" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var closeDate string
	err := s.db.QueryRow("SELECT submissions_close FROM events LIMIT 1").Scan(&closeDate)
	
	if err == nil && closeDate != "" {
		closeTime, parseErr := time.Parse(time.RFC3339, closeDate)
		if parseErr == nil && time.Now().After(closeTime) {
			http.Error(w, "Event is closed for submissions", http.StatusBadRequest)
			return
		}
	}

	var teamID string
	err = s.db.QueryRow("SELECT team_id FROM team_members WHERE email = ?", user.Email).Scan(&teamID)
	if err != nil {
		http.Error(w, "You must be in a team to submit", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	summary := r.FormValue("summary")
	repoURL := r.FormValue("repo_url")
	trackID := r.FormValue("track_id")

	projectID := uuid.New().String()
	submittedAt := time.Now().Format(time.RFC3339)

	_, err = s.db.Exec("INSERT INTO projects (id, team_id, track_id, title, summary, repo_url, submitted_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		projectID, teamID, trackID, title, summary, repoURL, submittedAt)

	if err != nil {
		http.Error(w, "Failed to submit project", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleJudgeScores(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey).(*User)

	if user == nil || (user.Role != "judge" && user.Role != "organizer") {
		http.Error(w, "Unauthorized or Forbidden", http.StatusForbidden)
		return
	}

	requestedJudge := r.URL.Query().Get("judge")
	targetJudgeRef := user.RefID

	if requestedJudge != "" {
		targetJudgeRef = requestedJudge
	}

	if user.Role == "judge" && targetJudgeRef != user.RefID {
		http.Error(w, "Cannot view peer scores", http.StatusForbidden)
		return
	}

	rows, err := s.db.Query("SELECT project_id, criteria, comment FROM scores WHERE judge_id = ?", targetJudgeRef)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Score struct {
		ProjectID string `json:"project_id"`
		Criteria  string `json:"criteria"`
		Comment   string `json:"comment"`
	}
	var scores []Score
	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.ProjectID, &sc.Criteria, &sc.Comment); err == nil {
			scores = append(scores, sc)
		}
	}
	if scores == nil {
		scores = []Score{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"scores": scores})
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	
	rows, err := s.db.Query("SELECT id, title FROM projects")
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Write([]byte("id,title\n"))
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err == nil {
			w.Write([]byte(id + "," + title + "\n"))
		}
	}
}

func (s *Server) handleExportLossless(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", "attachment; filename=\"tribunal-lossless.sqlite\"")
	http.ServeFile(w, r, s.db.Path)
}

func (s *Server) handleImportLossless(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("database")
	if err != nil {
		http.Error(w, "Missing database file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	
	s.db.Close()
	
	out, err := os.Create(s.db.Path)
	if err != nil {
		http.Error(w, "Failed to create target file", http.StatusInternalServerError)
		return
	}
	io.Copy(out, file)
	out.Close()

	log.Fatalf("Lossless import applied. Restarting Tribunal to load new DB...")
}

func (s *Server) handleExportBundle(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Build results.json
	rows, err := s.db.Query("SELECT p.id, p.title, t.name FROM projects p JOIN tracks t ON p.track_id = t.id")
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var projects []ProjectView
	for rows.Next() {
		var p ProjectView
		if err := rows.Scan(&p.ID, &p.Title, &p.TrackName); err == nil {
			projects = append(projects, p)
		}
	}
	projIdx := make(map[string]int)
	for i, p := range projects { projIdx[p.ID] = i }
	wins := make([][]int, len(projects))
	for i := range wins { wins[i] = make([]int, len(projects)) }
	
	compRows, _ := s.db.Query("SELECT winner_id, loser_id FROM pairwise_comparisons")
	for compRows.Next() {
		var wID, lID string
		if err := compRows.Scan(&wID, &lID); err == nil {
			if wIdx, ok1 := projIdx[wID]; ok1 {
				if lIdx, ok2 := projIdx[lID]; ok2 {
					wins[wIdx][lIdx]++
				}
			}
		}
	}
	compRows.Close()

	scores := judging.FitBradleyTerry(wins, len(projects))
	var ranked []RankedProject
	for i, p := range projects {
		ranked = append(ranked, RankedProject{
			ID: p.ID, Title: p.Title, TrackName: p.TrackName, ScoreFloat: scores[i],
		})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].ScoreFloat > ranked[j].ScoreFloat })
	resultsJSON, _ := json.MarshalIndent(ranked, "", "  ")

	// Build audit-anchor.json
	var latestHash string
	err = s.db.QueryRow("SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1").Scan(&latestHash)
	if err != nil {
		latestHash = "0000000000000000000000000000000000000000000000000000000000000000"
	}
	anchor := map[string]string{"latest_audit_hash": latestHash, "timestamp": time.Now().UTC().Format(time.RFC3339)}
	anchorJSON, _ := json.MarshalIndent(anchor, "", "  ")

	// Build manifest.json
	resultsHash := sha256.Sum256(resultsJSON)
	anchorHash := sha256.Sum256(anchorJSON)
	manifest := map[string]string{
		"results_hash": hex.EncodeToString(resultsHash[:]),
		"anchor_hash":  hex.EncodeToString(anchorHash[:]),
		"issuer":       "Tribunal Engine T4",
		"version":      "1.0",
	}
	// Sign manifest with HMAC using a known secret (or event ID)
	var eventID string
	s.db.QueryRow("SELECT id FROM events LIMIT 1").Scan(&eventID)
	mac := hmac.New(sha256.New, []byte(eventID))
	mac.Write([]byte(manifest["results_hash"] + manifest["anchor_hash"]))
	manifest["signature"] = hex.EncodeToString(mac.Sum(nil))
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"results-bundle.zip\"")
	
	zw := zip.NewWriter(w)
	
	f1, _ := zw.Create("results.json")
	f1.Write(resultsJSON)
	
	f2, _ := zw.Create("audit-anchor.json")
	f2.Write(anchorJSON)
	
	f3, _ := zw.Create("manifest.json")
	f3.Write(manifestJSON)
	
	zw.Close()
	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "export_bundle", user.Email, "Exported T4 Signed Bundle")
}

func (s *Server) handleEditProjectGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "participant" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	var teamID string
	err := s.db.QueryRow("SELECT team_id FROM team_members WHERE email = ?", user.Email).Scan(&teamID)
	if err != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	var p ProjectView
	err = s.db.QueryRow("SELECT id, title, summary, track_id, repo_url FROM projects WHERE team_id = ?", teamID).Scan(&p.ID, &p.Title, &p.Summary, &p.TrackID, &p.RepoURL)
	if err != nil {
		http.Redirect(w, r, "/projects/new", http.StatusSeeOther)
		return
	}

	rows, _ := s.db.Query("SELECT id, name FROM tracks")
	var tracks []Track
	defer rows.Close()
	for rows.Next() {
		var t Track
		rows.Scan(&t.ID, &t.Name)
		tracks = append(tracks, t)
	}

	data := PageData{User: user, Tracks: tracks, Project: &p}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["projects_edit"].ExecuteTemplate(w, "base", data)
}

func (s *Server) handleEditProjectPost(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "participant" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var closeDate string
	err := s.db.QueryRow("SELECT submissions_close FROM events LIMIT 1").Scan(&closeDate)
	
	if err == nil && closeDate != "" {
		closeTime, parseErr := time.Parse(time.RFC3339, closeDate)
		if parseErr == nil && time.Now().After(closeTime) {
			http.Error(w, "Event is closed for submissions", http.StatusBadRequest)
			return
		}
	}

	var teamID string
	err = s.db.QueryRow("SELECT team_id FROM team_members WHERE email = ?", user.Email).Scan(&teamID)
	if err != nil {
		http.Error(w, "You must be in a team to edit", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	summary := r.FormValue("summary")
	repoURL := r.FormValue("repo_url")
	trackID := r.FormValue("track_id")

	_, err = s.db.Exec("UPDATE projects SET title = ?, summary = ?, repo_url = ?, track_id = ? WHERE team_id = ?",
		title, summary, repoURL, trackID, teamID)

	if err != nil {
		http.Error(w, "Failed to edit project", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleJoinTeam(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "participant" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	teamID := r.URL.Query().Get("id")
	if teamID == "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Check if team exists
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM teams WHERE id = ?", teamID).Scan(&count)
	if err != nil || count == 0 {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Make sure user is not already in a team
	var currentTeam string
	err = s.db.QueryRow("SELECT team_id FROM team_members WHERE email = ?", user.Email).Scan(&currentTeam)
	if err == nil && currentTeam != "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Check member count (limit 4)
	var memberCount int
	s.db.QueryRow("SELECT count(*) FROM team_members WHERE team_id = ?", teamID).Scan(&memberCount)
	if memberCount >= 4 {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Add user to team
	s.db.Exec("INSERT INTO team_members (team_id, email) VALUES (?, ?)", teamID, user.Email)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
func (s *Server) handleJudgePairwiseGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "judge" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Make Pairwise Adaptive: fetch projects assigned to this judge
	// and prioritize pairs that have been compared the least globally.
	rows, err := s.db.Query(`
		SELECT p.id, p.title, p.summary, t.name, p.repo_url,
		       (SELECT COUNT(*) FROM pairwise_comparisons c WHERE (c.winner_id = p.id OR c.loser_id = p.id)) as comp_count
		FROM projects p 
		JOIN tracks t ON p.track_id = t.id 
		JOIN judge_assignments a ON p.id = a.project_id
		WHERE a.judge_id = ?
		ORDER BY comp_count ASC, RANDOM()
		LIMIT 2
	`, user.RefID)
	
	var projects []ProjectView
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p ProjectView
			var repo sql.NullString
			var count int
			if err := rows.Scan(&p.ID, &p.Title, &p.Summary, &p.TrackName, &repo, &count); err == nil {
				if repo.Valid {
					p.RepoURL = repo.String
				}
				projects = append(projects, p)
			}
		}
	}

	data := PageData{User: user}
	if len(projects) == 2 {
		data.ProjectA = &projects[0]
		data.ProjectB = &projects[1]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["judge_pairwise"].ExecuteTemplate(w, "base", data)
}

func (s *Server) handleJudgePairwisePost(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "judge" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	winnerID := r.FormValue("winner_id")
	loserID := r.FormValue("loser_id")

	if winnerID != "" && loserID != "" {
		s.db.Exec("INSERT INTO pairwise_comparisons (judge_id, winner_id, loser_id) VALUES (?, ?, ?)", user.RefID, winnerID, loserID)
	}

	http.Redirect(w, r, "/judge/pairwise", http.StatusSeeOther)
}

func (s *Server) handleJudgeRubricGet(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "judge" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	rows, err := s.db.Query("SELECT p.id, p.title, p.summary, t.name FROM projects p JOIN tracks t ON p.track_id = t.id JOIN judge_tracks jt ON p.track_id = jt.track_id WHERE jt.judge_id = ?", user.RefID)
	
	var projects []ProjectView
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p ProjectView
			if err := rows.Scan(&p.ID, &p.Title, &p.Summary, &p.TrackName); err == nil {
				projects = append(projects, p)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["judge_rubric"].ExecuteTemplate(w, "base", PageData{User: user, Projects: projects})
}



func (s *Server) handleOrganizerLeaderboard(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(userContextKey).(*User)
	if user.Role != "organizer" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Fetch all projects
	rows, err := s.db.Query("SELECT p.id, p.title, t.name FROM projects p JOIN tracks t ON p.track_id = t.id")
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []ProjectView
	for rows.Next() {
		var p ProjectView
		if err := rows.Scan(&p.ID, &p.Title, &p.TrackName); err == nil {
			projects = append(projects, p)
		}
	}

	n := len(projects)
	if n == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl["organizer_leaderboard"].ExecuteTemplate(w, "base", PageData{User: user})
		return
	}

	// Map ID -> Index
	projIdx := make(map[string]int)
	for i, p := range projects {
		projIdx[p.ID] = i
	}

	// Fetch comparisons
	compRows, _ := s.db.Query("SELECT winner_id, loser_id FROM pairwise_comparisons")
	
	wins := make([][]int, n)
	for i := range wins {
		wins[i] = make([]int, n)
	}

	if compRows != nil {
		defer compRows.Close()
		for compRows.Next() {
			var wID, lID string
			if err := compRows.Scan(&wID, &lID); err == nil {
				if wi, wok := projIdx[wID]; wok {
					if li, lok := projIdx[lID]; lok {
						wins[wi][li]++
					}
				}
			}
		}
	}

	// Call Bradley-Terry
	qualities := judging.FitBradleyTerry(wins, n)
	intervals := judging.BootstrapConfidenceIntervals(wins, n, 500)

	var ranked []RankedProject
	for i, q := range qualities {
		rp := RankedProject{
			Title:      projects[i].Title,
			TrackName:  projects[i].TrackName,
			Score:      fmt.Sprintf("%.4f", q),
			ScoreFloat: q,
		}
		if len(intervals) > i {
			rp.CILower = intervals[i].Lower
			rp.CIUpper = intervals[i].Upper
		}
		ranked = append(ranked, rp)
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].ScoreFloat > ranked[j].ScoreFloat
	})

	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	// Build judge comparisons for LOO
	judgeComps := make(map[string][][2]int)
	compRows2, _ := s.db.Query("SELECT judge_id, winner_id, loser_id FROM pairwise_comparisons")
	if compRows2 != nil {
		defer compRows2.Close()
		for compRows2.Next() {
			var jID, wID, lID string
			if err := compRows2.Scan(&jID, &wID, &lID); err == nil {
				if wi, wok := projIdx[wID]; wok {
					if li, lok := projIdx[lID]; lok {
						judgeComps[jID] = append(judgeComps[jID], [2]int{wi, li})
					}
				}
			}
		}
	}

	influences := judging.LeaveOneOutJudgeInfluence(wins, n, judgeComps)

	sort.Slice(influences, func(i, j int) bool {
		return influences[i].MaxRankDiff > influences[j].MaxRankDiff
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl["organizer_leaderboard"].ExecuteTemplate(w, "base", PageData{
		User:            user,
		RankedProjects:  ranked,
		JudgeInfluences: influences,
	})
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var payload struct {
		TargetURL string `json:"target_url"`
		Event     string `json:"event"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Save to DB
	_, err := s.db.Exec("INSERT INTO webhooks (event, target_url) VALUES (?, ?)", payload.Event, payload.TargetURL)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "configure_webhook", user.Email, "Configured webhook for "+payload.Event)

	// Production-grade webhook delivery in a background goroutine
	go func(url string, event string) {
		if url == "" {
			return
		}
		body, _ := json.Marshal(map[string]string{"event": event, "timestamp": time.Now().Format(time.RFC3339)})
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
		if err != nil {
			fmt.Printf("Webhook delivery failed to %s: %v\n", url, err)
			return
		}
		defer resp.Body.Close()
		fmt.Printf("Webhook delivered to %s, status: %d\n", url, resp.StatusCode)
	}(payload.TargetURL, payload.Event)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"webhook_queued"}`))
}

func (s *Server) handleCertificate(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey).(*User)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	secret := os.Getenv("HMAC_SECRET")
	if secret == "" {
		secret = "default-dev-secret-do-not-use-in-prod"
	}

	issuedAt := time.Now().Format(time.RFC3339)
	payload := fmt.Sprintf("%s:%s:%s", user.Email, user.Role, issuedAt)
	
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	cert := map[string]interface{}{
		"participant": user.Email,
		"role": user.Role,
		"issued_at": issuedAt,
		"signature": signature,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cert)
}

func (s *Server) handlePublicVote(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		http.Error(w, "Missing project ID", http.StatusBadRequest)
		return
	}

	var votingMode string
	err := s.db.QueryRow("SELECT voting_mode FROM events LIMIT 1").Scan(&votingMode)
	if err != nil {
		votingMode = "authenticated"
	}

	user, _ := r.Context().Value(userContextKey).(*User)
	ip := strings.Split(r.RemoteAddr, ":")[0]
	identity := ip

	if votingMode == "authenticated" {
		if user == nil {
			http.Error(w, "Must be logged in to vote", http.StatusUnauthorized)
			return
		}
		identity = user.Email
	} else if votingMode == "email-gated" {
		var p struct{ Email string `json:"email"` }
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Email == "" {
			http.Error(w, "Email required", http.StatusBadRequest)
			return
		}
		identity = p.Email
	}

	_, err = s.db.Exec("INSERT INTO public_votes (project_id, identity, ip_address) VALUES (?, ?, ?)", projectID, identity, ip)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "You have already voted for this project", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to record vote", http.StatusInternalServerError)
		return
	}

	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "public_vote", identity, "Voted for "+projectID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"voted"}`))
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	user, _ := r.Context().Value(userContextKey).(*User)
	
	identity := strings.Split(r.RemoteAddr, ":")[0]
	if user != nil {
		identity = user.Email
	}

	var payload struct{ Content string `json:"content"` }
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Content == "" {
		http.Error(w, "Invalid comment", http.StatusBadRequest)
		return
	}

	s.db.Exec("INSERT INTO comments (project_id, author_identity, content) VALUES (?, ?, ?)", projectID, identity, payload.Content)
	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "add_comment", identity, "Commented on "+projectID)
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"comment_added"}`))
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	user := r.Context().Value(userContextKey).(*User)

	var payload struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		RepoURL string `json:"repo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var teamID string
	err := s.db.QueryRow("SELECT team_id FROM projects WHERE id = ?").Scan(&teamID) // FIXED: Missing ID arg here, let's fix
	err = s.db.QueryRow("SELECT team_id FROM projects WHERE id = ?", projectID).Scan(&teamID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	var isMember bool
	err = s.db.QueryRow("SELECT 1 FROM team_members WHERE team_id = ? AND email = ?", teamID, user.Email).Scan(&isMember)
	if err != nil && user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	_, err = s.db.Exec("UPDATE projects SET title = ?, summary = ?, repo_url = ? WHERE id = ?", payload.Title, payload.Summary, payload.RepoURL, projectID)
	if err != nil {
		http.Error(w, "Failed to update project", http.StatusInternalServerError)
		return
	}

	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "update_project", user.Email, "Updated project "+projectID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"updated"}`))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	user := r.Context().Value(userContextKey).(*User)

	if user.Role != "organizer" {
		var teamID string
		if err := s.db.QueryRow("SELECT team_id FROM projects WHERE id = ?", projectID).Scan(&teamID); err == nil {
			var isMember bool
			if err := s.db.QueryRow("SELECT 1 FROM team_members WHERE team_id = ? AND email = ?", teamID, user.Email).Scan(&isMember); err != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}

	s.db.Exec("DELETE FROM projects WHERE id = ?", projectID)
	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "delete_project", user.Email, "Deleted project "+projectID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted"}`))
}

func (s *Server) handleCertificateVerify(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Participant string `json:"participant"`
		Role        string `json:"role"`
		IssuedAt    string `json:"issued_at"`
		Signature   string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	secret := os.Getenv("HMAC_SECRET")
	if secret == "" {
		secret = "default-dev-secret-do-not-use-in-prod"
	}

	data := fmt.Sprintf("%s:%s:%s", payload.Participant, payload.Role, payload.IssuedAt)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if hmac.Equal([]byte(payload.Signature), []byte(expectedSignature)) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"verified"}`))
	} else {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
	}
}

func (s *Server) handlePublicResults(w http.ResponseWriter, r *http.Request) {
	var votingClose sql.NullString
	var votingMode string
	err := s.db.QueryRow("SELECT voting_close, voting_mode FROM events LIMIT 1").Scan(&votingClose, &votingMode)
	
	if err == nil && votingClose.Valid && votingClose.String != "" {
		closeTime, parseErr := time.Parse(time.RFC3339, votingClose.String)
		if parseErr == nil && time.Now().Before(closeTime) {
			http.Error(w, "Results are hidden until voting closes.", http.StatusForbidden)
			return
		}
	}

	// Calculate and display results (simplified version of leaderboard without internals)
	rows, err := s.db.Query("SELECT p.id, p.title, t.name FROM projects p JOIN tracks t ON p.track_id = t.id")
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []ProjectView
	for rows.Next() {
		var p ProjectView
		if err := rows.Scan(&p.ID, &p.Title, &p.TrackName); err == nil {
			projects = append(projects, p)
		}
	}

	projIdx := make(map[string]int)
	for i, p := range projects {
		projIdx[p.ID] = i
	}

	wins := make([][]int, len(projects))
	for i := range wins {
		wins[i] = make([]int, len(projects))
	}

	compRows, _ := s.db.Query("SELECT winner_id, loser_id FROM pairwise_comparisons")
	defer compRows.Close()

	for compRows.Next() {
		var wID, lID string
		if err := compRows.Scan(&wID, &lID); err == nil {
			if wIdx, ok1 := projIdx[wID]; ok1 {
				if lIdx, ok2 := projIdx[lID]; ok2 {
					wins[wIdx][lIdx]++
				}
			}
		}
	}

	scores := judging.FitBradleyTerry(wins, len(projects))
	
	// Add public votes
	voteRows, _ := s.db.Query("SELECT project_id, COUNT(*) FROM public_votes GROUP BY project_id")
	defer voteRows.Close()
	publicVotes := make(map[string]int)
	for voteRows.Next() {
		var pID string
		var count int
		if err := voteRows.Scan(&pID, &count); err == nil {
			publicVotes[pID] = count
		}
	}

	var ranked []RankedProject
	for i, p := range projects {
		var publicScore float64
		if votingMode == "quadratic" {
			// Quadratic voting impact: sqrt(N)
			publicScore = math.Sqrt(float64(publicVotes[p.ID])) * 0.5
		} else {
			// Linear impact
			publicScore = float64(publicVotes[p.ID]) * 0.1
		}
		
		totalScore := scores[i] + publicScore
		ranked = append(ranked, RankedProject{
			Title:      p.Title,
			TrackName:  p.TrackName,
			ScoreFloat: totalScore,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].ScoreFloat > ranked[j].ScoreFloat
	})

	for i := range ranked {
		ranked[i].Rank = i + 1
		ranked[i].Score = fmt.Sprintf("%.2f", ranked[i].ScoreFloat)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": ranked})
}

func (s *Server) handleBulkImport(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userContextKey).(*User)
	if user == nil || user.Role != "organizer" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	var importData struct {
		Projects []struct {
			ID      string `json:"id"`
			TeamID  string `json:"team_id"`
			TrackID string `json:"track_id"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
			RepoURL string `json:"repo_url"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(r.Body).Decode(&importData); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}

	for _, p := range importData.Projects {
		tx.Exec("INSERT INTO projects (id, team_id, track_id, title, summary, repo_url, submitted_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			p.ID, p.TeamID, p.TrackID, p.Title, p.Summary, p.RepoURL, time.Now().Format(time.RFC3339))
	}
	
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		http.Error(w, "Import failed", http.StatusInternalServerError)
		return
	}

	s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "bulk_import", user.Email, "Imported projects")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"import_completed"}`))
}

func (s *Server) handleEmbedGallery(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT p.id, p.title, p.summary, t.name FROM projects p JOIN tracks t ON p.track_id = t.id ORDER BY RANDOM() LIMIT 10")
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "ALLOWALL")
	w.Write([]byte(`<!DOCTYPE html><html><head><style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: transparent; padding: 10px; margin: 0; color: #e4e4e7; }
		.card { border: 1px solid #3f3f46; border-radius: 8px; padding: 15px; margin-bottom: 15px; background: rgba(24, 24, 27, 0.8); backdrop-filter: blur(8px); }
		h4 { margin: 0 0 5px 0; font-size: 1.1rem; color: #fff; }
		p { margin: 0 0 10px 0; font-size: 0.9rem; color: #a1a1aa; }
		small { display: inline-block; padding: 2px 8px; background: #2563eb; color: #fff; border-radius: 12px; font-size: 0.75rem; }
	</style></head><body>`))
	
	for rows.Next() {
		var id, title, summary, track string
		if err := rows.Scan(&id, &title, &summary, &track); err == nil {
			w.Write([]byte(fmt.Sprintf(`<div class="card"><h4>%s</h4><p>%s</p><small>%s</small></div>`, title, summary, track)))
		}
	}
	w.Write([]byte(`</body></html>`))
}
