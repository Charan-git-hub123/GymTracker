package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	_ "github.com/lib/pq"
)

var db *sql.DB

// ---------------- MODELS ----------------

type MuscleGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	MuscleGroup string `json:"muscle_group"`
}

type Set struct {
	SetNumber int `json:"set_number"`
	Weight    int `json:"weight"`
	Reps      int `json:"reps"`
}

// ---------------- MAIN ----------------

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

	// Routes
	http.HandleFunc("/muscle-groups", cors(getMuscleGroups))
	http.HandleFunc("/add-muscle", cors(addMuscle))
	http.HandleFunc("/delete-muscle", cors(deleteMuscle))

	http.HandleFunc("/exercises", cors(getExercises))
	http.HandleFunc("/add-exercise", cors(addExercise))
	http.HandleFunc("/delete-exercise", cors(deleteExercise))

	http.HandleFunc("/log-workout", cors(logWorkout))
	http.HandleFunc("/exercise-details", cors(getExerciseDetails))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ---------------- CORS ----------------

func cors(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		handler(w, r)
	}
}

// ---------------- MUSCLE ----------------

func getMuscleGroups(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, name FROM muscle_groups")

	var result []MuscleGroup

	for rows.Next() {
		var m MuscleGroup
		rows.Scan(&m.ID, &m.Name)
		result = append(result, m)
	}

	json.NewEncoder(w).Encode(result)
}

func addMuscle(w http.ResponseWriter, r *http.Request) {
	var m MuscleGroup
	json.NewDecoder(r.Body).Decode(&m)

	_, err := db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", m.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("added"))
}

func deleteMuscle(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, err := db.Exec("DELETE FROM muscle_groups WHERE id=$1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("deleted"))
}

// ---------------- EXERCISES ----------------

func getExercises(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("muscle_id")

	rows, _ := db.Query(`
		SELECT e.id, e.name, m.name
		FROM exercises e
		JOIN muscle_groups m ON e.muscle_group_id = m.id
		WHERE m.id = $1
	`, id)

	var result []Exercise

	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name, &e.MuscleGroup)
		result = append(result, e)
	}

	json.NewEncoder(w).Encode(result)
}

func addExercise(w http.ResponseWriter, r *http.Request) {
	type Input struct {
		Name string `json:"name"`
		ID   int    `json:"muscle_id"`
	}

	var input Input
	json.NewDecoder(r.Body).Decode(&input)

	_, err := db.Exec(
		"INSERT INTO exercises(name, muscle_group_id) VALUES($1,$2)",
		input.Name, input.ID,
	)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("added"))
}

func deleteExercise(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, err := db.Exec("DELETE FROM exercises WHERE id=$1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("deleted"))
}

// ---------------- WORKOUT ----------------

func logWorkout(w http.ResponseWriter, r *http.Request) {
	type Input struct {
		ExerciseID int    `json:"exercise_id"`
		Date       string `json:"date"`
		Sets       []Set  `json:"sets"`
	}

	var input Input
	json.NewDecoder(r.Body).Decode(&input)

	tx, _ := db.Begin()

	var logID int

	tx.QueryRow(
		"INSERT INTO exercise_logs(exercise_id, workout_date) VALUES($1,$2) RETURNING id",
		input.ExerciseID, input.Date,
	).Scan(&logID)

	for _, s := range input.Sets {
		tx.Exec(
			"INSERT INTO sets(exercise_log_id,set_number,weight,reps) VALUES($1,$2,$3,$4)",
			logID, s.SetNumber, s.Weight, s.Reps,
		)
	}

	tx.Commit()

	w.Write([]byte("logged"))
}

// ---------------- DETAILS ----------------

func getExerciseDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)

	rows, _ := db.Query(`
		SELECT weight, reps
		FROM sets s
		JOIN exercise_logs el ON s.exercise_log_id = el.id
		WHERE el.exercise_id = $1
		ORDER BY weight DESC
		LIMIT 5
	`, id)

	type Result struct {
		Weight int `json:"weight"`
		Reps   int `json:"reps"`
	}

	var result []Result

	for rows.Next() {
		var r Result
		rows.Scan(&r.Weight, &r.Reps)
		result = append(result, r)
	}

	json.NewEncoder(w).Encode(result)
}