// Small web app for practicing Kubernetes: counts visits in PostgreSQL.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webapp_http_requests_total",
		Help: "HTTP requests to the visit counter, by status code.",
	}, []string{"code"})
	latency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "webapp_http_request_duration_seconds",
		Help: "Latency of requests to the visit counter, including the database round trips.",
	}, []string{})
)

func main() {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable",
		os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_HOST"), os.Getenv("DB_NAME"))
	hostname, _ := os.Hostname()

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	defer pool.Close()

	ensureSchema := func(ctx context.Context) error {
		_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS visits (
			id SERIAL PRIMARY KEY, pod TEXT NOT NULL, at TIMESTAMPTZ NOT NULL DEFAULT now())`)
		return err
	}

	counter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := ensureSchema(ctx); err != nil {
			http.Error(w, "db error: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		if _, err := pool.Exec(ctx, `INSERT INTO visits (pod) VALUES ($1)`, hostname); err != nil {
			http.Error(w, "db error: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		var total int
		var version string
		pool.QueryRow(ctx, `SELECT count(*) FROM visits`).Scan(&total)
		pool.QueryRow(ctx, `SHOW server_version`).Scan(&version)
		fmt.Fprintf(w, "Hello from pod %s\nTotal visits (stored in PostgreSQL %s): %d\n", hostname, version, total)
	})
	http.Handle("/", promhttp.InstrumentHandlerDuration(latency, promhttp.InstrumentHandlerCounter(requests, counter)))
	http.Handle("/metrics", promhttp.Handler())

	// Liveness: the process is up. Readiness: the database is reachable.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "db not reachable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
