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

// ---------------- MODELS ----------------

type MuscleGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	MuscleGroup string `json:"muscle_group"`
	YoutubeLink string `json:"youtube_link"`
}

type ExerciseLog struct {
	ID           int    `json:"id"`
	ExerciseName string `json:"exercise_name"`
	Date         string `json:"date"`
}

// ---------------- MAIN ----------------

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

	http.HandleFunc("/", homeHandler)

	http.HandleFunc("/muscle-groups", getMuscleGroups)
	http.HandleFunc("/add-muscle-group", addMuscleGroup)

	http.HandleFunc("/exercises", getExercises)
	http.HandleFunc("/add-exercise", addExercise)

	http.HandleFunc("/workout-history", getWorkoutHistory)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("🚀 Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ---------------- UI PAGE ----------------

func homeHandler(w http.ResponseWriter, r *http.Request) {
	html := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Gym Tracker</title>
	</head>
	<body>

	<h1>🏋️ Gym Tracker</h1>

	<h2>Add Muscle Group</h2>
	<input id="mgName" placeholder="Muscle Group">
	<button onclick="addMuscleGroup()">Add</button>

	<h2>Add Exercise</h2>
	<input id="exName" placeholder="Exercise Name">
	<input id="mgID" placeholder="Muscle Group ID">
	<input id="yt" placeholder="YouTube Link">
	<button onclick="addExercise()">Add</button>

	<h2>View Exercises</h2>
	<button onclick="getExercises()">Load</button>

	<h2>Workout History</h2>
	<button onclick="getHistory()">Load</button>

	<pre id="output"></pre>

	<script>

	function addMuscleGroup(){
		fetch('/add-muscle-group', {
			method:'POST',
			headers:{'Content-Type':'application/json'},
			body: JSON.stringify({name: document.getElementById('mgName').value})
		}).then(()=>alert('Added'))
	}

	function addExercise(){
		fetch('/add-exercise', {
			method:'POST',
			headers:{'Content-Type':'application/json'},
			body: JSON.stringify({
				name: document.getElementById('exName').value,
				muscle_group_id: parseInt(document.getElementById('mgID').value),
				youtube_link: document.getElementById('yt').value
			})
		}).then(()=>alert('Exercise Added'))
	}

	function getExercises(){
		fetch('/exercises')
		.then(res=>res.json())
		.then(data=>{
			document.getElementById('output').innerText = JSON.stringify(data,null,2)
		})
	}

	function getHistory(){
		fetch('/workout-history')
		.then(res=>res.json())
		.then(data=>{
			document.getElementById('output').innerText = JSON.stringify(data,null,2)
		})
	}

	</script>

	</body>
	</html>
	`

	fmt.Fprintln(w, html)
}

// ---------------- BACKEND ----------------

func getMuscleGroups(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, name FROM muscle_groups")
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

	db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", input.Name)
}

func getExercises(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`
		SELECT e.id, e.name, m.name, COALESCE(e.youtube_link, '')
		FROM exercises e
		JOIN muscle_groups m ON e.muscle_group_id = m.id
	`)
	defer rows.Close()

	var list []Exercise

	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name, &e.MuscleGroup, &e.YoutubeLink)
		list = append(list, e)
	}

	json.NewEncoder(w).Encode(list)
}

func addExercise(w http.ResponseWriter, r *http.Request) {
	type Input struct {
		Name          string
		MuscleGroupID int
		YoutubeLink   string
	}

	var input Input
	json.NewDecoder(r.Body).Decode(&input)

	db.Exec(
		"INSERT INTO exercises(name, muscle_group_id, youtube_link) VALUES($1,$2,$3)",
		input.Name, input.MuscleGroupID, input.YoutubeLink,
	)
}

func getWorkoutHistory(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query(`
		SELECT el.id, e.name, el.workout_date
		FROM exercise_logs el
		JOIN exercises e ON el.exercise_id = e.id
		ORDER BY el.workout_date DESC
	`)
	defer rows.Close()

	var logs []ExerciseLog

	for rows.Next() {
		var l ExerciseLog
		rows.Scan(&l.ID, &l.ExerciseName, &l.Date)
		logs = append(logs, l)
	}

	json.NewEncoder(w).Encode(logs)
}