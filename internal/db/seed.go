package db

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

type Fixture struct {
	Event    Event     `json:"event"`
	Tracks   []Track   `json:"tracks"`
	Judges   []Judge   `json:"judges"`
	Teams    []Team    `json:"teams"`
	Projects []Project `json:"projects"`
	Scores   []Score   `json:"scores"`
}

type Event struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	SubmissionsClose string `json:"submissions_close"`
}

type Track struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Judge struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Tracks []string `json:"tracks"`
}

type Team struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type Project struct {
	ID          string `json:"id"`
	Team        string `json:"team"`
	Track       string `json:"track"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	RepoURL     string `json:"repo_url"`
	SubmittedAt string `json:"submitted_at"`
}

type Score struct {
	Judge    string                 `json:"judge"`
	Project  string                 `json:"project"`
	Criteria map[string]interface{} `json:"criteria"`
	Comment  string                 `json:"comment"`
}

func (db *DB) LoadFixtures(filepath string) error {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		log.Printf("Skipping fixtures loading: %v", err)
		return nil
	}

	var fix Fixture
	if err := json.Unmarshal(data, &fix); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing (optional, but good for idempotent load)
	tables := []string{"webhooks", "audit_logs", "comments", "public_votes", "pairwise_comparisons", "sessions", "users", "scores", "projects", "team_members", "teams", "judge_tracks", "judges", "tracks", "events"}
	for _, t := range tables {
		tx.Exec("DELETE FROM " + t)
	}

	// Insert event
	if fix.Event.ID != "" {
		_, err = tx.Exec("INSERT INTO events (id, name, submissions_close) VALUES (?, ?, ?)",
			fix.Event.ID, fix.Event.Name, fix.Event.SubmissionsClose)
		if err != nil {
			return err
		}
	}

	// Tracks
	for _, t := range fix.Tracks {
		_, err = tx.Exec("INSERT INTO tracks (id, name) VALUES (?, ?)", t.ID, t.Name)
		if err != nil {
			return err
		}
	}

	// Judges
	for _, j := range fix.Judges {
		_, err = tx.Exec("INSERT INTO judges (id, name, email) VALUES (?, ?, ?)", j.ID, j.Name, j.Email)
		if err != nil {
			return err
		}
		for _, tID := range j.Tracks {
			_, err = tx.Exec("INSERT INTO judge_tracks (judge_id, track_id) VALUES (?, ?)", j.ID, tID)
			if err != nil {
				return err
			}
		}
	}

	// Teams
	for _, t := range fix.Teams {
		_, err = tx.Exec("INSERT INTO teams (id, name) VALUES (?, ?)", t.ID, t.Name)
		if err != nil {
			return err
		}
		for _, m := range t.Members {
			_, err = tx.Exec("INSERT INTO team_members (team_id, email) VALUES (?, ?)", t.ID, m)
			if err != nil {
				return err
			}
		}
	}

	// Projects
	for _, p := range fix.Projects {
		_, err = tx.Exec("INSERT INTO projects (id, team_id, track_id, title, summary, repo_url, submitted_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			p.ID, p.Team, p.Track, p.Title, p.Summary, p.RepoURL, p.SubmittedAt)
		if err != nil {
			return err
		}
	}

	// Scores
	for _, s := range fix.Scores {
		criteriaJSON, _ := json.Marshal(s.Criteria)
		_, err = tx.Exec("INSERT INTO scores (judge_id, project_id, criteria, comment) VALUES (?, ?, ?, ?)",
			s.Judge, s.Project, string(criteriaJSON), s.Comment)
		if err != nil {
			return err
		}
	}

	// Insert standard auth users and sessions for the checker
	users := []struct {
		ID, Email, Hash, Role, RefID string
	}{
		{"user_org", "org@example.org", "$2a$10$wN9Q/xN0xO3yHhHw0n2D/OIfcQ5U9b9w0U.p8h6i0zQjQ0F3eZ.y6", "organizer", "org"},
		{"user_jdg_a", "tomas.varga@example.org", "$2a$10$wN9Q/xN0xO3yHhHw0n2D/OIfcQ5U9b9w0U.p8h6i0zQjQ0F3eZ.y6", "judge", "jdg_01"},
		{"user_jdg_b", "wei.lindqvist@example.org", "$2a$10$wN9Q/xN0xO3yHhHw0n2D/OIfcQ5U9b9w0U.p8h6i0zQjQ0F3eZ.y6", "judge", "jdg_02"},
		{"user_prt", "priya1@example.org", "$2a$10$wN9Q/xN0xO3yHhHw0n2D/OIfcQ5U9b9w0U.p8h6i0zQjQ0F3eZ.y6", "participant", "priya1@example.org"},
	}
	for _, u := range users {
		_, err = tx.Exec("INSERT INTO users (id, email, password_hash, role, ref_id) VALUES (?, ?, ?, ?, ?)",
			u.ID, u.Email, u.Hash, u.Role, u.RefID)
		if err != nil {
			return err
		}
	}

	sessions := []struct {
		Token, UserID string
	}{
		{"org_7f2a", "user_org"},
		{"jdg_a_91bc", "user_jdg_a"},
		{"jdg_b_44de", "user_jdg_b"},
		{"prt_2e88", "user_prt"},
	}
	for _, s := range sessions {
		_, err = tx.Exec("INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, datetime('now', '+30 days'))",
			s.Token, s.UserID)
		if err != nil {
			return err
		}
	}

	// Output the auth strings as required by the spec
	log.Println("organizer   = \"Cookie: session=org_7f2a\"")
	log.Println("judge_a     = \"Cookie: session=jdg_a_91bc\"")
	log.Println("judge_b     = \"Cookie: session=jdg_b_44de\"")
	log.Println("participant = \"Cookie: session=prt_2e88\"")

	return tx.Commit()
}
