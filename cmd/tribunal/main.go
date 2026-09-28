package main

import (
	"log"
	"net/http"
	"os"

	"github.com/hackathon-raptors/tribunal/internal/db"
	"github.com/hackathon-raptors/tribunal/internal/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "dogfood.sqlite"
	}

	log.Printf("Starting Tribunal on :%s, DB: %s", port, dbPath)

	database := db.InitDB(dbPath)
	defer database.Close()

	if fixturePath := os.Getenv("FIXTURES_PATH"); fixturePath != "" {
		if err := database.LoadFixtures(fixturePath); err != nil {
			log.Printf("Failed to load fixtures: %v", err)
		} else {
			log.Printf("Loaded fixtures from %s", fixturePath)
		}
	}

	srv := server.NewServer(database)
	log.Fatal(http.ListenAndServe(":"+port, srv.Router()))
}
