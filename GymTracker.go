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

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cors(w)

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// ---------- MUSCLES ----------
func getMuscles(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, name FROM muscle_groups")
	defer rows.Close()

	muscles := []map[string]interface{}{}

	for rows.Next() {
		var id int
		var name string
		rows.Scan(&id, &name)
		muscles = append(muscles, map[string]interface{}{
			"id":   id,
			"name": name,
		})
	}

	json.NewEncoder(w).Encode(muscles)
}

// ---------- EXERCISES ----------
func getExercises(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("muscle_id")

	rows, _ := db.Query("SELECT id, name FROM exercises WHERE muscle_group_id=$1", id)
	defer rows.Close()

	exercises := []map[string]interface{}{}

	for rows.Next() {
		var id int
		var name string
		rows.Scan(&id, &name)
		exercises = append(exercises, map[string]interface{}{
			"id":   id,
			"name": name,
		})
	}

	json.NewEncoder(w).Encode(exercises)
}

func addExercise(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	json.NewDecoder(r.Body).Decode(&data)

	db.Exec("INSERT INTO exercises(name, muscle_group_id) VALUES($1,$2)",
		data["name"], data["muscle_group_id"])

	w.Write([]byte("ok"))
}

// ---------- WORKOUT ----------
func addWorkout(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	json.NewDecoder(r.Body).Decode(&data)

	var logID int
	err := db.QueryRow(
		"INSERT INTO exercise_logs(exercise_id) VALUES($1) RETURNING id",
		data["exercise_id"],
	).Scan(&logID)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	sets := data["sets"].([]interface{})

	for _, s := range sets {
		set := s.(map[string]interface{})

		db.Exec(`INSERT INTO sets(exercise_log_id,set_number,weight,reps)
		 VALUES($1,$2,$3,$4)`,
			logID,
			set["set_number"],
			set["weight"],
			set["reps"],
		)
	}

	w.Write([]byte("saved"))
}

// ---------- HISTORY ----------
func getHistory(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("exercise_id")

	rows, _ := db.Query(`
		SELECT s.set_number, s.weight, s.reps, e.workout_date
		FROM sets s
		JOIN exercise_logs e ON s.exercise_log_id = e.id
		WHERE e.exercise_id=$1
		ORDER BY e.workout_date DESC
	`, id)

	defer rows.Close()

	var result []string

	for rows.Next() {
		var set, weight, reps int
		var date string

		rows.Scan(&set, &weight, &reps, &date)

		result = append(result,
			date+" - Set "+string(rune(set))+
				" | "+string(rune(weight))+"kg x "+string(rune(reps)))
	}

	json.NewEncoder(w).Encode(result)
}

func main() {
	conn := os.Getenv("DATABASE_URL")

	var err error
	db, err = sql.Open("postgres", conn)
	if err != nil {
		log.Fatal(err)
	}

	db.Ping()

	http.HandleFunc("/muscles", middleware(getMuscles))
	http.HandleFunc("/exercises", middleware(getExercises))
	http.HandleFunc("/add-exercise", middleware(addExercise))
	http.HandleFunc("/add-workout", middleware(addWorkout))
	http.HandleFunc("/history", middleware(getHistory))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Running on", port)
	http.ListenAndServe(":"+port, nil)
}