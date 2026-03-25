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

func main() {
	var err error

	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/muscle-groups", musclesHandler)
	http.HandleFunc("/exercises", exercisesHandler)
	http.HandleFunc("/exercise-details/", exerciseDetails)
	http.HandleFunc("/log-workout", logWorkout)
	http.HandleFunc("/delete", deleteHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on", port)
	log.Fatal(http.ListenAndServe(":"+port, cors(http.DefaultServeMux)))
}

// -------- CORS --------
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")

		if r.Method == "OPTIONS" {
			return
		}
		h.ServeHTTP(w, r)
	})
}

// -------- MUSCLES --------
func musclesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, _ := db.Query("SELECT id, name FROM muscle_groups")
		defer rows.Close()

		var res []map[string]interface{}
		for rows.Next() {
			var id int
			var name string
			rows.Scan(&id, &name)

			res = append(res, map[string]interface{}{
				"id": id, "name": name,
			})
		}
		json.NewEncoder(w).Encode(res)
	}

	if r.Method == "POST" {
		var data map[string]string
		json.NewDecoder(r.Body).Decode(&data)

		db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", data["name"])
		w.Write([]byte("ok"))
	}
}

// -------- EXERCISES --------
func exercisesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, _ := db.Query(`
			SELECT e.id, e.name, m.name
			FROM exercises e
			JOIN muscle_groups m ON e.muscle_group_id = m.id
		`)
		defer rows.Close()

		var res []map[string]interface{}
		for rows.Next() {
			var id int
			var name, muscle string
			rows.Scan(&id, &name, &muscle)

			res = append(res, map[string]interface{}{
				"id": id, "name": name, "muscle": muscle,
			})
		}
		json.NewEncoder(w).Encode(res)
	}

	if r.Method == "POST" {
		var data map[string]interface{}
		json.NewDecoder(r.Body).Decode(&data)

		db.Exec(
			"INSERT INTO exercises(name, muscle_group_id) VALUES($1,$2)",
			data["name"], int(data["muscle_group_id"].(float64)),
		)
		w.Write([]byte("ok"))
	}
}

// -------- EXERCISE DETAILS (PR + RECENT) --------
func exerciseDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/exercise-details/"):]
	id, _ := strconv.Atoi(idStr)

	// PR
	var pr int
	db.QueryRow(`
		SELECT COALESCE(MAX(weight),0)
		FROM sets s
		JOIN exercise_logs el ON s.exercise_log_id = el.id
		WHERE el.exercise_id = $1
	`, id).Scan(&pr)

	// Recent
	rows, _ := db.Query(`
		SELECT el.workout_time, s.weight, s.reps
		FROM sets s
		JOIN exercise_logs el ON s.exercise_log_id = el.id
		WHERE el.exercise_id = $1
		ORDER BY el.workout_time DESC
		LIMIT 5
	`, id)
	defer rows.Close()

	var recent []map[string]interface{}
	for rows.Next() {
		var time string
		var wgt, reps int
		rows.Scan(&time, &wgt, &reps)

		recent = append(recent, map[string]interface{}{
			"time": time, "weight": wgt, "reps": reps,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"pr":     pr,
		"recent": recent,
	})
}

// -------- LOG WORKOUT --------
func logWorkout(w http.ResponseWriter, r *http.Request) {
	var data struct {
		ExerciseID int `json:"exercise_id"`
		Sets       []struct {
			Weight int `json:"weight"`
			Reps   int `json:"reps"`
		} `json:"sets"`
	}

	json.NewDecoder(r.Body).Decode(&data)

	var logID int
	db.QueryRow(
		"INSERT INTO exercise_logs(exercise_id) VALUES($1) RETURNING id",
		data.ExerciseID,
	).Scan(&logID)

	for i, s := range data.Sets {
		db.Exec(
			"INSERT INTO sets(exercise_log_id,set_number,weight,reps) VALUES($1,$2,$3,$4)",
			logID, i+1, s.Weight, s.Reps,
		)
	}

	w.Write([]byte("saved"))
}

// -------- DELETE --------
func deleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	table := r.URL.Query().Get("type")

	query := "DELETE FROM " + table + " WHERE id=$1"
	db.Exec(query, id)

	w.Write([]byte("deleted"))
}