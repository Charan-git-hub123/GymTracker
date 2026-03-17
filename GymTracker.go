package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

var db *sql.DB

// -------------------- MODELS --------------------

type MuscleGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	MuscleGroup  string `json:"muscle_group"`
	YoutubeLink  string `json:"youtube_link"`
}

type ExerciseLog struct {
	ID           int    `json:"id"`
	ExerciseName string `json:"exercise_name"`
	Date         string `json:"date"`
}

type Set struct {
	SetNumber int `json:"set_number"`
	Weight    int `json:"weight"`
	Reps      int `json:"reps"`
}

// -------------------- MAIN --------------------

func main() {
	var err error

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err = sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("DB connection failed:", err)
	}

	fmt.Println("✅ Connected to DB")

	// Routes
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/muscle-groups", getMuscleGroups)
	http.HandleFunc("/add-muscle-group", addMuscleGroup)

	http.HandleFunc("/exercises", getExercises)
	http.HandleFunc("/add-exercise", addExercise)

	http.HandleFunc("/log-workout", logWorkout)
	http.HandleFunc("/workout-history", getWorkoutHistory)

	fmt.Println("🚀 Server running on port 8080")
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// -------------------- HANDLERS --------------------

func homeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Gym Tracker API Running")
}

// -------------------- MUSCLE GROUPS --------------------

func getMuscleGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name FROM muscle_groups")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var groups []MuscleGroup

	for rows.Next() {
		var g MuscleGroup
		rows.Scan(&g.ID, &g.Name)
		groups = append(groups, g)
	}

	json.NewEncoder(w).Encode(groups)
}

func addMuscleGroup(w http.ResponseWriter, r *http.Request) {
	var input MuscleGroup
	json.NewDecoder(r.Body).Decode(&input)

	_, err := db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", input.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintln(w, "Muscle group added")
}

// -------------------- EXERCISES --------------------

func getExercises(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT e.id, e.name, m.name, COALESCE(e.youtube_link, '')
		FROM exercises e
		JOIN muscle_groups m ON e.muscle_group_id = m.id
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var exercises []Exercise

	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name, &e.MuscleGroup, &e.YoutubeLink)
		exercises = append(exercises, e)
	}

	json.NewEncoder(w).Encode(exercises)
}

func addExercise(w http.ResponseWriter, r *http.Request) {
	type Input struct {
		Name          string `json:"name"`
		MuscleGroupID int    `json:"muscle_group_id"`
		YoutubeLink   string `json:"youtube_link"`
	}

	var input Input
	json.NewDecoder(r.Body).Decode(&input)

	_, err := db.Exec(
		"INSERT INTO exercises(name, muscle_group_id, youtube_link) VALUES($1, $2, $3)",
		input.Name, input.MuscleGroupID, input.YoutubeLink,
	)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintln(w, "Exercise added")
}

// -------------------- LOG WORKOUT --------------------

func logWorkout(w http.ResponseWriter, r *http.Request) {
	type Input struct {
		ExerciseID int    `json:"exercise_id"`
		Date       string `json:"date"`
		Sets       []Set  `json:"sets"`
	}

	var input Input
	json.NewDecoder(r.Body).Decode(&input)

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var logID int

	err = tx.QueryRow(
		"INSERT INTO exercise_logs(exercise_id, workout_date) VALUES($1, $2) RETURNING id",
		input.ExerciseID, input.Date,
	).Scan(&logID)

	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}

	for _, s := range input.Sets {
		_, err := tx.Exec(
			"INSERT INTO sets(exercise_log_id, set_number, weight, reps) VALUES($1, $2, $3, $4)",
			logID, s.SetNumber, s.Weight, s.Reps,
		)
		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), 500)
			return
		}
	}

	tx.Commit()
	fmt.Fprintln(w, "Workout logged")
}

// -------------------- WORKOUT HISTORY --------------------

func getWorkoutHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT el.id, e.name, el.workout_date
		FROM exercise_logs el
		JOIN exercises e ON el.exercise_id = e.id
		ORDER BY el.workout_date DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var logs []ExerciseLog

	for rows.Next() {
		var l ExerciseLog
		rows.Scan(&l.ID, &l.ExerciseName, &l.Date)
		logs = append(logs, l)
	}

	json.NewEncoder(w).Encode(logs)
}