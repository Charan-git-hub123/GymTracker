package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	//"strconv"

	_ "github.com/lib/pq"
)

var db *sql.DB

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
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

	var muscles []map[string]interface{}

	for rows.Next() {
		var id int
		var name string
		rows.Scan(&id, &name)
		muscles = append(muscles, map[string]interface{}{
			"id": id, "name": name,
		})
	}

	json.NewEncoder(w).Encode(muscles)
}

func addMuscle(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	json.NewDecoder(r.Body).Decode(&data)

	db.Exec("INSERT INTO muscle_groups(name) VALUES($1)", data["name"])
	w.Write([]byte("ok"))
}

// ---------- EXERCISES ----------
func getExercises(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("muscle_id")

	rows, _ := db.Query(
		"SELECT id, name, description, youtube_link FROM exercises WHERE muscle_group_id=$1", id)
	defer rows.Close()

	var exercises []map[string]interface{}

	for rows.Next() {
		var id int
		var name, desc, link string
		rows.Scan(&id, &name, &desc, &link)

		exercises = append(exercises, map[string]interface{}{
			"id": id, "name": name,
			"description": desc,
			"youtube_link": link,
		})
	}

	json.NewEncoder(w).Encode(exercises)
}

func addExercise(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	json.NewDecoder(r.Body).Decode(&data)

	db.Exec(`INSERT INTO exercises(name, description, youtube_link, muscle_group_id)
	VALUES($1,$2,$3,$4)`,
		data["name"], data["description"], data["youtube_link"], data["muscle_group_id"])

	w.Write([]byte("ok"))
}

func updateExercise(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	json.NewDecoder(r.Body).Decode(&data)

	db.Exec(`UPDATE exercises SET name=$1, description=$2, youtube_link=$3 WHERE id=$4`,
		data["name"], data["description"], data["youtube_link"], data["id"])

	w.Write([]byte("updated"))
}

// ---------- WORKOUT ----------
func addWorkout(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	json.NewDecoder(r.Body).Decode(&data)

	var logID int
	db.QueryRow(
		"INSERT INTO exercise_logs(exercise_id) VALUES($1) RETURNING id",
		data["exercise_id"],
	).Scan(&logID)

	sets := data["sets"].([]interface{})

	for _, s := range sets {
		set := s.(map[string]interface{})

		db.Exec(`INSERT INTO sets(exercise_log_id,set_number,weight,reps)
		VALUES($1,$2,$3,$4)`,
			logID,
			set["set_number"],
			set["weight"],
			set["reps"])
	}

	w.Write([]byte("saved"))
}

func deleteMuscle(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	db.Exec("DELETE FROM muscle_groups WHERE id=$1", id)

	w.Write([]byte("deleted"))
}

func deleteExercise(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	db.Exec("DELETE FROM exercises WHERE id=$1", id)

	w.Write([]byte("deleted"))
}

func deleteWorkout(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("log_id")

	db.Exec("DELETE FROM exercise_logs WHERE id=$1", id)

	w.Write([]byte("deleted"))
}



func deleteSet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("set_id")

	db.Exec("DELETE FROM sets WHERE id=$1", id)

	w.Write([]byte("deleted"))
}
// ---------- HISTORY GROUPED BY DATE ----------
func getHistory(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("exercise_id")

	rows, _ := db.Query(`
		SELECT e.workout_date, s.set_number, s.weight, s.reps
		FROM exercise_logs e
		JOIN sets s ON e.id = s.exercise_log_id
		WHERE e.exercise_id=$1
		ORDER BY e.workout_date DESC, s.set_number
	`, id)

	defer rows.Close()

	result := make(map[string][]map[string]int)

	for rows.Next() {
		var date string
		var set, weight, reps int
		rows.Scan(&date, &set, &weight, &reps)

		result[date] = append(result[date], map[string]int{
			"set": set,
			"weight": weight,
			"reps": reps,
		})
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
	http.HandleFunc("/add-muscle", middleware(addMuscle))
	http.HandleFunc("/exercises", middleware(getExercises))
	http.HandleFunc("/add-exercise", middleware(addExercise))
	http.HandleFunc("/update-exercise", middleware(updateExercise))
	http.HandleFunc("/add-workout", middleware(addWorkout))
	http.HandleFunc("/history", middleware(getHistory))
	http.HandleFunc("/delete-muscle", middleware(deleteMuscle))
	http.HandleFunc("/delete-exercise", middleware(deleteExercise))
	http.HandleFunc("/delete-workout", middleware(deleteWorkout))
	http.HandleFunc("/delete-set", middleware(deleteSet))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Running on", port)
	http.ListenAndServe(":"+port, nil)
}