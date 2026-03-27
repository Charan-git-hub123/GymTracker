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

// -------------------- CORS --------------------
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// -------------------- MODELS --------------------
type MuscleGroup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	MuscleGroupID int    `json:"muscle_group_id"`
}

type Set struct {
	ID        int `json:"id"`
	ExerciseID int `json:"exercise_id"`
	Weight    int `json:"weight"`
	Reps      int `json:"reps"`
}

// -------------------- HANDLERS --------------------

// GET muscle groups
func getMuscleGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name FROM muscle_groups")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var list []MuscleGroup
	for rows.Next() {
		var m MuscleGroup
		rows.Scan(&m.ID, &m.Name)
		list = append(list, m)
	}

	json.NewEncoder(w).Encode(list)
}

// ADD muscle group
func addMuscleGroup(w http.ResponseWriter, r *http.Request) {
	var m MuscleGroup
	json.NewDecoder(r.Body).Decode(&m)

	_, err := db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", m.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("added"))
}

// DELETE muscle group
func deleteMuscleGroup(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, err := db.Exec("DELETE FROM muscle_groups WHERE id=$1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("deleted"))
}

// GET exercises by muscle
func getExercises(w http.ResponseWriter, r *http.Request) {
	mid := r.URL.Query().Get("muscle_id")

	rows, err := db.Query("SELECT id, name, muscle_group_id FROM exercises WHERE muscle_group_id=$1", mid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var list []Exercise
	for rows.Next() {
		var e Exercise
		rows.Scan(&e.ID, &e.Name, &e.MuscleGroupID)
		list = append(list, e)
	}

	json.NewEncoder(w).Encode(list)
}

// ADD exercise
func addExercise(w http.ResponseWriter, r *http.Request) {
	var e Exercise
	json.NewDecoder(r.Body).Decode(&e)

	_, err := db.Exec("INSERT INTO exercises(name, muscle_group_id) VALUES($1,$2)", e.Name, e.MuscleGroupID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("added"))
}

// DELETE exercise
func deleteExercise(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	_, err := db.Exec("DELETE FROM exercises WHERE id=$1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("deleted"))
}

// ADD set
func addSet(w http.ResponseWriter, r *http.Request) {
	var s Set
	json.NewDecoder(r.Body).Decode(&s)

	_, err := db.Exec("INSERT INTO sets(exercise_id, weight, reps) VALUES($1,$2,$3)",
		s.ExerciseID, s.Weight, s.Reps)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("set added"))
}

// GET sets by exercise
func getSets(w http.ResponseWriter, r *http.Request) {
	eid := r.URL.Query().Get("exercise_id")

	rows, err := db.Query("SELECT id, exercise_id, weight, reps FROM sets WHERE exercise_id=$1", eid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var list []Set
	for rows.Next() {
		var s Set
		rows.Scan(&s.ID, &s.ExerciseID, &s.Weight, &s.Reps)
		list = append(list, s)
	}

	json.NewEncoder(w).Encode(list)
}

// -------------------- MAIN --------------------
func main() {

	var err error

	connStr := os.Getenv("DATABASE_URL")
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	// ROUTES
	http.HandleFunc("/muscle-groups", enableCORS(getMuscleGroups))
	http.HandleFunc("/add-muscle", enableCORS(addMuscleGroup))
	http.HandleFunc("/delete-muscle", enableCORS(deleteMuscleGroup))

	http.HandleFunc("/exercises", enableCORS(getExercises))
	http.HandleFunc("/add-exercise", enableCORS(addExercise))
	http.HandleFunc("/delete-exercise", enableCORS(deleteExercise))

	http.HandleFunc("/sets", enableCORS(getSets))
	http.HandleFunc("/add-set", enableCORS(addSet))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on port", port)
	http.ListenAndServe(":"+port, nil)
}