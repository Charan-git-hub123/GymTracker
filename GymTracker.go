package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var db *sql.DB

type Exercise struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	MuscleGroupID int    `json:"muscle_group_id"`
	YoutubeLink   string `json:"youtube_link"`
}

func main() {
	connStr := os.Getenv("DATABASE_URL")
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()

	// CORS
	r.Use(mux.CORSMethodMiddleware(r))
	r.Use(corsMiddleware)

	r.HandleFunc("/exercises", getExercises).Methods("GET")
	r.HandleFunc("/exercises", createExercise).Methods("POST")

	log.Println("Server running on :8080")
	http.ListenAndServe(":8080", r)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if r.Method == "OPTIONS" {
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getExercises(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT e.id, e.name, e.muscle_group_id, e.youtube_link
		FROM exercises e
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var exercises []Exercise

	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name, &e.MuscleGroupID, &e.YoutubeLink)
		exercises = append(exercises, e)
	}

	json.NewEncoder(w).Encode(exercises)
}

func createExercise(w http.ResponseWriter, r *http.Request) {
	var e Exercise
	json.NewDecoder(r.Body).Decode(&e)

	err := db.QueryRow(
		`INSERT INTO exercises (name, muscle_group_id, youtube_link)
		 VALUES ($1, $2, $3) RETURNING id`,
		e.Name, e.MuscleGroupID, e.YoutubeLink,
	).Scan(&e.ID)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(e)
}