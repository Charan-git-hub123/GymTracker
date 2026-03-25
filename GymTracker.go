package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

var db *sql.DB

type Muscle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	var err error

	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/muscle-groups", handleMuscles)
	http.HandleFunc("/exercises", handleExercises)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on", port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(http.DefaultServeMux)))
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

func handleMuscles(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, _ := db.Query("SELECT id, name FROM muscle_groups")
		defer rows.Close()

		var list []Muscle

		for rows.Next() {
			var m Muscle
			rows.Scan(&m.ID, &m.Name)
			list = append(list, m)
		}

		json.NewEncoder(w).Encode(list)
		return
	}

	if r.Method == "POST" {
		var m Muscle
		json.NewDecoder(r.Body).Decode(&m)

		db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", m.Name)

		w.Write([]byte("ok"))
	}
}

func handleExercises(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var data struct {
			Name          string `json:"name"`
			MuscleGroupID int    `json:"muscle_group_id"`
		}

		json.NewDecoder(r.Body).Decode(&data)

		db.Exec(
			"INSERT INTO exercises(name, muscle_group_id) VALUES($1, $2)",
			data.Name, data.MuscleGroupID,
		)

		w.Write([]byte("ok"))
	}
}