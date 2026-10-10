// - empty lists serialise as [] rather than null
package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

//go:embed ui
var uiFiles embed.FS

var db *sql.DB

const queryTimeout = 10 * time.Second

// ---------- types ----------

type Muscle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Exercise struct {
	ID            int    `json:"id"`
	MuscleGroupID int    `json:"muscle_group_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	YouTubeLink   string `json:"youtube_link"`
}

type Set struct {
	ID        int     `json:"set_id"`
	SetNumber int     `json:"set_number"`
	Weight    float64 `json:"weight"`
	Reps      int     `json:"reps"`
}

type WorkoutLog struct {
	LogID int    `json:"log_id"`
	Date  string `json:"date"`
	Sets  []Set  `json:"sets"`
}

type ProgressPoint struct {
	Date      string  `json:"date"`
	TopWeight float64 `json:"top_weight"`
	Volume    float64 `json:"volume"`
	Sets      int     `json:"sets"`
}

// ---------- plumbing ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[warn] encode response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if status >= 500 {
		log.Printf("[error] %s", msg)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// cors keeps the original permissive policy: this is a single personal instance.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/assets") {
			log.Printf("%s %s %v", r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Millisecond))
		}
	})
}

// pathID reads a {id} path value and rejects anything non-numeric, so a bad URL
// is a 400 rather than a database error.
func pathID(r *http.Request) (int, error) {
	raw := r.PathValue("id")
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id %q", raw)
	}
	return id, nil
}

func queryID(r *http.Request, key string) (int, error) {
	raw := r.URL.Query().Get(key)
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("missing or invalid %s", key)
	}
	return id, nil
}

func ctx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), queryTimeout)
}

// ---------- muscle groups ----------

func listMuscles(w http.ResponseWriter, r *http.Request) {
	c, cancel := ctx(r)
	defer cancel()

	rows, err := db.QueryContext(c, "SELECT id, name FROM muscle_groups ORDER BY name")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list muscle groups: %v", err)
		return
	}
	defer rows.Close()

	out := []Muscle{} // never nil: encodes as [] not null
	for rows.Next() {
		var m Muscle
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan muscle group: %v", err)
			return
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "read muscle groups: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func createMuscle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	c, cancel := ctx(r)
	defer cancel()

	var m Muscle
	err := db.QueryRowContext(c,
		`INSERT INTO muscle_groups(name) VALUES($1)
		 ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id, name`, body.Name).Scan(&m.ID, &m.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create muscle group: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func deleteMuscle(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	res, err := db.ExecContext(c, "DELETE FROM muscle_groups WHERE id=$1", id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete muscle group: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "no muscle group with id %d", id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// ---------- exercises ----------

func listExercises(w http.ResponseWriter, r *http.Request) {
	muscleID, err := queryID(r, "muscle_id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	rows, err := db.QueryContext(c,
		`SELECT id, muscle_group_id, name, description, youtube_link
		 FROM exercises WHERE muscle_group_id=$1 ORDER BY name`, muscleID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list exercises: %v", err)
		return
	}
	defer rows.Close()

	out := []Exercise{}
	for rows.Next() {
		var e Exercise
		// NULL description / youtube_link used to make the scan fail, the error
		// was discarded, and the exercise silently disappeared from the list.
		var desc, link sql.NullString
		if err := rows.Scan(&e.ID, &e.MuscleGroupID, &e.Name, &desc, &link); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan exercise: %v", err)
			return
		}
		e.Description = desc.String
		e.YouTubeLink = link.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "read exercises: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func createExercise(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		YouTubeLink   string `json:"youtube_link"`
		MuscleGroupID int    `json:"muscle_group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || body.MuscleGroupID <= 0 {
		writeErr(w, http.StatusBadRequest, "name and muscle_group_id are required")
		return
	}

	c, cancel := ctx(r)
	defer cancel()

	var e Exercise
	err := db.QueryRowContext(c,
		`INSERT INTO exercises(name, description, youtube_link, muscle_group_id)
		 VALUES($1,$2,$3,$4) RETURNING id, muscle_group_id, name`,
		body.Name, body.Description, body.YouTubeLink, body.MuscleGroupID,
	).Scan(&e.ID, &e.MuscleGroupID, &e.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create exercise: %v", err)
		return
	}
	e.Description = body.Description
	e.YouTubeLink = body.YouTubeLink
	writeJSON(w, http.StatusCreated, e)
}

func deleteExercise(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	res, err := db.ExecContext(c, "DELETE FROM exercises WHERE id=$1", id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete exercise: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "no exercise with id %d", id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// ---------- workouts ----------

func createWorkout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExerciseID int    `json:"exercise_id"`
		Date       string `json:"date"` // optional YYYY-MM-DD
		Sets       []Set  `json:"sets"`
	}
	// Typed decoding: the previous version did data["sets"].([]interface{}),
	// which panicked whenever the key was missing or the wrong shape.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return
	}
	if body.ExerciseID <= 0 {
		writeErr(w, http.StatusBadRequest, "exercise_id is required")
		return
	}
	if len(body.Sets) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one set is required")
		return
	}
	for i, s := range body.Sets {
		if s.Reps < 0 || s.Weight < 0 {
			writeErr(w, http.StatusBadRequest, "set %d: weight and reps cannot be negative", i+1)
			return
		}
	}
	if body.Date != "" {
		if _, err := time.Parse("2006-01-02", body.Date); err != nil {
			writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
	}

	c, cancel := ctx(r)
	defer cancel()

	// One transaction: a half-written workout is worse than a rejected one.
	tx, err := db.BeginTx(c, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "begin transaction: %v", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var logID int
	if body.Date == "" {
		err = tx.QueryRowContext(c,
			`INSERT INTO exercise_logs(exercise_id) VALUES($1) RETURNING id`,
			body.ExerciseID).Scan(&logID)
	} else {
		err = tx.QueryRowContext(c,
			`INSERT INTO exercise_logs(exercise_id, workout_date) VALUES($1,$2) RETURNING id`,
			body.ExerciseID, body.Date).Scan(&logID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create workout: %v", err)
		return
	}

	for i, s := range body.Sets {
		num := s.SetNumber
		if num == 0 {
			num = i + 1
		}
		if _, err := tx.ExecContext(c,
			`INSERT INTO sets(exercise_log_id, set_number, weight, reps) VALUES($1,$2,$3,$4)`,
			logID, num, s.Weight, s.Reps); err != nil {
			writeErr(w, http.StatusInternalServerError, "save set %d: %v", i+1, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, "commit workout: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"log_id": logID, "sets": len(body.Sets)})
}

func deleteWorkout(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	res, err := db.ExecContext(c, "DELETE FROM exercise_logs WHERE id=$1", id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete workout: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "no workout with id %d", id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func deleteSet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	res, err := db.ExecContext(c, "DELETE FROM sets WHERE id=$1", id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "delete set: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "no set with id %d", id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// ---------- history & progress ----------

func history(w http.ResponseWriter, r *http.Request) {
	exerciseID, err := queryID(r, "exercise_id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	rows, err := db.QueryContext(c,
		`SELECT l.id, to_char(l.workout_date, 'YYYY-MM-DD'), s.id, s.set_number, s.weight, s.reps
		 FROM exercise_logs l
		 JOIN sets s ON s.exercise_log_id = l.id
		 WHERE l.exercise_id = $1
		 ORDER BY l.workout_date DESC, l.id DESC, s.set_number`, exerciseID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "read history: %v", err)
		return
	}
	defer rows.Close()

	out := []WorkoutLog{}
	byLog := map[int]int{} // log id -> index in out
	for rows.Next() {
		var logID, setID, setNum, reps int
		var date string
		var weight float64
		if err := rows.Scan(&logID, &date, &setID, &setNum, &weight, &reps); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan history: %v", err)
			return
		}
		idx, seen := byLog[logID]
		if !seen {
			out = append(out, WorkoutLog{LogID: logID, Date: date, Sets: []Set{}})
			idx = len(out) - 1
			byLog[logID] = idx
		}
		out[idx].Sets = append(out[idx].Sets, Set{ID: setID, SetNumber: setNum, Weight: weight, Reps: reps})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "read history: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// progress powers the chart: one point per session, oldest first.
func progress(w http.ResponseWriter, r *http.Request) {
	exerciseID, err := queryID(r, "exercise_id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "%v", err)
		return
	}
	c, cancel := ctx(r)
	defer cancel()

	rows, err := db.QueryContext(c,
		`SELECT to_char(l.workout_date, 'YYYY-MM-DD') AS d,
		        MAX(s.weight)         AS top_weight,
		        SUM(s.weight * s.reps) AS volume,
		        COUNT(*)              AS set_count
		 FROM exercise_logs l
		 JOIN sets s ON s.exercise_log_id = l.id
		 WHERE l.exercise_id = $1
		 GROUP BY d
		 ORDER BY d`, exerciseID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "read progress: %v", err)
		return
	}
	defer rows.Close()

	out := []ProgressPoint{}
	for rows.Next() {
		var p ProgressPoint
		if err := rows.Scan(&p.Date, &p.TopWeight, &p.Volume, &p.Sets); err != nil {
			writeErr(w, http.StatusInternalServerError, "scan progress: %v", err)
			return
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "read progress: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func health(w http.ResponseWriter, r *http.Request) {
	c, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(c); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unreachable: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------- main ----------

func openDB(dsn string) (*sql.DB, error) {
	if !strings.Contains(dsn, "connect_timeout=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "connect_timeout=10"
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)
	conn.SetConnMaxLifetime(5 * time.Minute)
	conn.SetConnMaxIdleTime(time.Minute)

	c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.PingContext(c); err != nil {
		return nil, fmt.Errorf("ping: %v", err)
	}
	return conn, nil
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	var err error
	// The original ignored both sql.Open's error and Ping's result, so the app
	// started happily against an unreachable database and failed per request.
	db, err = openDB(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/muscles", listMuscles)
	mux.HandleFunc("POST /api/muscles", createMuscle)
	mux.HandleFunc("DELETE /api/muscles/{id}", deleteMuscle)

	mux.HandleFunc("GET /api/exercises", listExercises)
	mux.HandleFunc("POST /api/exercises", createExercise)
	mux.HandleFunc("DELETE /api/exercises/{id}", deleteExercise)

	mux.HandleFunc("POST /api/workouts", createWorkout)
	mux.HandleFunc("DELETE /api/workouts/{id}", deleteWorkout)
	mux.HandleFunc("DELETE /api/sets/{id}", deleteSet)

	mux.HandleFunc("GET /api/history", history)
	mux.HandleFunc("GET /api/progress", progress)
	mux.HandleFunc("GET /healthz", health)

	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		log.Fatalf("embedded ui: %v", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(ui)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(cors(mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		log.Printf("GymTracker listening on :%s", port)
		// The original discarded this error, so a port clash exited silently.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}