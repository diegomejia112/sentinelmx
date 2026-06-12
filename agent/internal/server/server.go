package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/diegomejia11/sentinelmx/agent/internal/telemetry"
)

type Alert struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Severity  string    `json:"severity"`
}

type AlertStore struct {
	mu     sync.RWMutex
	alerts [100]Alert
	index  int
	count  int
}

func (as *AlertStore) Add(a Alert) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.alerts[as.index] = a
	as.index = (as.index + 1) % 100
	if as.count < 100 {
		as.count++
	}
}

func (as *AlertStore) GetAll() []Alert {
	as.mu.RLock()
	defer as.mu.RUnlock()
	result := make([]Alert, 0, as.count)
	if as.count == 0 {
		return result
	}
	start := (as.index - as.count + 100) % 100
	for i := 0; i < as.count; i++ {
		result = append(result, as.alerts[(start+i)%100])
	}
	return result
}

type Server struct {
	store    *AlertStore
	port     int
	alertsCh chan Alert
}

func NewServer(port int) *Server {
	return &Server{
		store:    &AlertStore{},
		port:     port,
		alertsCh: make(chan Alert, 50),
	}
}

// AddAlert permite que el monitor de telemetría inyecte alertas al servidor
func (s *Server) AddAlert(msg, severity string) {
	a := Alert{Message: msg, Timestamp: time.Now(), Severity: severity}
	s.store.Add(a)
	select {
	case s.alertsCh <- a:
	default: // no bloquear si no hay suscriptores
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := telemetry.CollectMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.store.GetAll())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m, err := telemetry.CollectMetrics()
			if err != nil {
				continue
			}
			// Formato que el dashboard espera
			payload := map[string]any{
				"type": "system_metrics",
				"cpu":  m.CPUUsagePercent,
				"ram":  m.MemUsedPercent,
				"swap": float64(m.SwapUsedKB) / 1024.0,
			}
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case alert := <-s.alertsCh:
			payload := map[string]any{
				"type":      "alert",
				"message":   alert.Message,
				"timestamp": alert.Timestamp,
				"severity":  alert.Severity,
			}
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "event: alert\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/alerts", s.handleAlerts)
	mux.HandleFunc("/api/events", s.handleEvents)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: cors(mux),
	}

	log.Printf("SentinelMX API en http://localhost:%d", s.port)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}
