package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var db *sql.DB

type Muscle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Muscle  string `json:"muscle"`
}

type Set struct {
	Weight int `json:"weight"`
	Reps   int `json:"reps"`
}

func main() {
	var err error
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()

	// CORS FIX
	r.Use(corsMiddleware)

	// ROUTES
	r.HandleFunc("/muscle-groups", getMuscles).Methods("GET")
	r.HandleFunc("/muscle-groups", addMuscle).Methods("POST")
	r.HandleFunc("/muscle-groups/{id}", deleteMuscle).Methods("DELETE")
	r.HandleFunc("/muscle-groups/{id}", updateMuscle).Methods("PUT")

	r.HandleFunc("/exercises", getExercises).Methods("GET")
	r.HandleFunc("/exercises", addExercise).Methods("POST")
	r.HandleFunc("/exercises/{id}", deleteExercise).Methods("DELETE")
	r.HandleFunc("/exercises/{id}", updateExercise).Methods("PUT")

	r.HandleFunc("/exercise-details/{id}", getDetails).Methods("GET")
	r.HandleFunc("/log-workout", logWorkout).Methods("POST")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Running on", port)
	http.ListenAndServe(":"+port, r)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == "OPTIONS" {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------- MUSCLE ----------------

func getMuscles(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, name FROM muscle_groups")
	var res []Muscle
	for rows.Next() {
		var m Muscle
		rows.Scan(&m.ID, &m.Name)
		res = append(res, m)
	}
	json.NewEncoder(w).Encode(res)
}

func addMuscle(w http.ResponseWriter, r *http.Request) {
	var m Muscle
	json.NewDecoder(r.Body).Decode(&m)
	db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", m.Name)
}

func deleteMuscle(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	db.Exec("DELETE FROM muscle_groups WHERE id=$1", id)
}

func updateMuscle(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var m Muscle
	json.NewDecoder(r.Body).Decode(&m)
	db.Exec("UPDATE muscle_groups SET name=$1 WHERE id=$2", m.Name, id)
}

// ---------------- EXERCISE ----------------

func getExercises(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`
		SELECT e.id, e.name, m.name
		FROM exercises e
		JOIN muscle_groups m ON e.muscle_group_id = m.id
	`)

	var res []Exercise
	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name, &e.Muscle)
		res = append(res, e)
	}
	json.NewEncoder(w).Encode(res)
}

func addExercise(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name          string `json:"name"`
		MuscleGroupID int    `json:"muscle_group_id"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	db.Exec("INSERT INTO exercises(name, muscle_group_id) VALUES($1,$2)",
		input.Name, input.MuscleGroupID)
}

func deleteExercise(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	db.Exec("DELETE FROM exercises WHERE id=$1", id)
}

func updateExercise(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var input struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&input)
	db.Exec("UPDATE exercises SET name=$1 WHERE id=$2", input.Name, id)
}

// ---------------- DETAILS ----------------

func getDetails(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	var pr int
	db.QueryRow(`
		SELECT COALESCE(MAX(weight),0)
		FROM sets s
		JOIN exercise_logs l ON s.exercise_log_id = l.id
		WHERE l.exercise_id=$1
	`, id).Scan(&pr)

	rows, _ := db.Query(`
		SELECT weight, reps
		FROM sets s
		JOIN exercise_logs l ON s.exercise_log_id = l.id
		WHERE l.exercise_id=$1
		ORDER BY l.workout_date DESC
		LIMIT 5
	`, id)

	var recent []Set
	for rows.Next() {
		var s Set
		rows.Scan(&s.Weight, &s.Reps)
		recent = append(recent, s)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"pr":     pr,
		"recent": recent,
	})
}

// ---------------- WORKOUT ----------------

func logWorkout(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExerciseID int   `json:"exercise_id"`
		Sets       []Set `json:"sets"`
	}
	json.NewDecoder(r.Body).Decode(&input)

	var logID int
	db.QueryRow(
		"INSERT INTO exercise_logs(exercise_id) VALUES($1) RETURNING id",
		input.ExerciseID,
	).Scan(&logID)

	for i, s := range input.Sets {
		db.Exec(
			"INSERT INTO sets(exercise_log_id,set_number,weight,reps) VALUES($1,$2,$3,$4)",
			logID, i+1, s.Weight, s.Reps,
		)
	}
}