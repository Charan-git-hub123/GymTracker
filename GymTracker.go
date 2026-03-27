package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

// ---------------- CORS ----------------
func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		next(w, r)
	}
}

// ---------------- MODELS ----------------
type Muscle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Set struct {
	SetNumber int `json:"set_number"`
	Weight    int `json:"weight"`
	Reps      int `json:"reps"`
}

// ---------------- MUSCLES ----------------
func getMuscles(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, name FROM muscle_groups")

	var list []Muscle
	for rows.Next() {
		var m Muscle
		rows.Scan(&m.ID, &m.Name)
		list = append(list, m)
	}

	json.NewEncoder(w).Encode(list)
}

func addMuscle(w http.ResponseWriter, r *http.Request) {
	var m Muscle
	json.NewDecoder(r.Body).Decode(&m)

	db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", m.Name)
}

// ---------------- EXERCISES ----------------
func getExercises(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("muscle_id")

	rows, _ := db.Query("SELECT id, name FROM exercises WHERE muscle_group_id=$1", id)

	var list []Exercise
	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name)
		list = append(list, e)
	}

	json.NewEncoder(w).Encode(list)
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

// ---------------- HISTORY ----------------
func getHistory(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("exercise_id")

	rows, _ := db.Query(`
		SELECT DISTINCT workout_date 
		FROM exercise_logs 
		WHERE exercise_id=$1
		ORDER BY workout_date DESC
	`, id)

	var dates []string
	for rows.Next() {
		var d string
		rows.Scan(&d)
		dates = append(dates, d)
	}

	json.NewEncoder(w).Encode(dates)
}

// ---------------- ADD WORKOUT ----------------
func addWorkout(w http.ResponseWriter, r *http.Request) {

	var input struct {
		ExerciseID int   `json:"exercise_id"`
		Sets       []Set `json:"sets"`
	}

	json.NewDecoder(r.Body).Decode(&input)

	tx, _ := db.Begin()

	var logID int
	tx.QueryRow(
		"INSERT INTO exercise_logs(exercise_id, workout_date) VALUES($1,$2) RETURNING id",
		input.ExerciseID, time.Now().Format("2006-01-02"),
	).Scan(&logID)

	for _, s := range input.Sets {
		tx.Exec(
			"INSERT INTO sets(exercise_log_id,set_number,weight,reps) VALUES($1,$2,$3,$4)",
			logID, s.SetNumber, s.Weight, s.Reps,
		)
	}

	tx.Commit()
}

// ---------------- MAIN ----------------
func main() {

	conn := os.Getenv("DATABASE_URL")
	db, _ = sql.Open("postgres", conn)

	http.HandleFunc("/muscles", cors(getMuscles))
	http.HandleFunc("/add-muscle", cors(addMuscle))

	http.HandleFunc("/exercises", cors(getExercises))
	http.HandleFunc("/add-exercise", cors(addExercise))

	http.HandleFunc("/history", cors(getHistory))
	http.HandleFunc("/add-workout", cors(addWorkout))

	log.Println("Server running...")
	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}