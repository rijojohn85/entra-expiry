package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
	"github.com/rijojohn85/entra-expiry/internal/report"
)

type Snapshot struct {
	Rows      []report.Row   `json:"rows"`
	Summary   report.Summary `json:"summary"`
	Timestamp time.Time      `json:"timestamp"`
}

type Server struct {
	port     int
	days     int
	interval time.Duration
	apps     AppProvider
	snap     atomic.Pointer[Snapshot]
}

// AppProvider is the consumer-side interface (see
// go.dev/wiki/CodeReviewComments#interfaces): the server depends only on
// this, tests use a fake, internal/provider stays concrete.
type AppProvider interface {
	ListApplications(ctx context.Context) ([]provider.Application, error)
}

var _ AppProvider = (*provider.GraphProvider)(nil)

func NewServer(port int, days int, interval time.Duration, apps AppProvider) *Server {
	return &Server{
		port:     port,
		days:     days,
		interval: interval,
		apps:     apps,
	}
}

func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/metrics", s.metrics)
	mux.HandleFunc("/report", s.report)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(s.port),
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go s.snapLoop(ctx)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting server", "port", s.port)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	snap := s.snap.Load()
	if snap == nil {
		http.Error(w, "no snapshot available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(snap.Summary)
	if err != nil {
		http.Error(w, "failed to encode snapshot", http.StatusInternalServerError)
		return
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	snap := s.snap.Load()
	if snap == nil {
		http.Error(w, "no snapshot available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(snap)
	if err != nil {
		http.Error(w, "failed to encode snapshot", http.StatusInternalServerError)
		return
	}
}

func (s *Server) snapLoop(ctx context.Context) {
	s.refresh(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh(ctx)
		}
	}
}

func (s *Server) refresh(ctx context.Context) {
	apps, err := s.apps.ListApplications(ctx)
	if err != nil {
		slog.Error("scan failed, keeping previous snapshot", "err", err)
		return
	}
	rows, summary := report.Build(apps, s.days, time.Now())
	s.snap.Store(&Snapshot{Rows: rows, Summary: summary, Timestamp: time.Now()})
	slog.Info("snapshot updated", "applications", len(apps), "summary", summary)
}
