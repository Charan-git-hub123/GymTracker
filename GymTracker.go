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

// ✅ CORS FIX (VERY IMPORTANT)
func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// Handle OPTIONS (preflight)
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)

		if r.Method == "OPTIONS" {
			return
		}

		next(w, r)
	}
}

type Muscle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ✅ GET MUSCLES
func getMuscles(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	rows, err := db.Query("SELECT id, name FROM muscle_groups")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var muscles []Muscle
	for rows.Next() {
		var m Muscle
		rows.Scan(&m.ID, &m.Name)
		muscles = append(muscles, m)
	}

	json.NewEncoder(w).Encode(muscles)
}

// ✅ ADD MUSCLE
func addMuscle(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	var m Muscle
	json.NewDecoder(r.Body).Decode(&m)

	_, err := db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", m.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("Added"))
}

// ✅ DELETE MUSCLE
func deleteMuscle(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	id := r.URL.Query().Get("id")

	_, err := db.Exec("DELETE FROM muscle_groups WHERE id=$1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("Deleted"))
}

func main() {

	// ✅ USE RENDER ENV VARIABLE (BEST WAY)
	connStr := os.Getenv("DATABASE_URL")

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/muscles", corsMiddleware(getMuscles))
	http.HandleFunc("/add-muscle", corsMiddleware(addMuscle))
	http.HandleFunc("/delete-muscle", corsMiddleware(deleteMuscle))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on port", port)
	http.ListenAndServe(":"+port, nil)
}