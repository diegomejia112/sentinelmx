package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/diegomejia11/sentinelmx/agent/internal/procmon"
	"github.com/diegomejia11/sentinelmx/agent/internal/storage"
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
	if as.count < 100 { as.count++ }
}

func (as *AlertStore) GetAll() []Alert {
	as.mu.RLock()
	defer as.mu.RUnlock()
	if as.count == 0 { return []Alert{} }
	result := make([]Alert, as.count)
	start := (as.index - as.count + 100) % 100
	for i := 0; i < as.count; i++ {
		result[i] = as.alerts[(start+i)%100]
	}
	return result
}

type Server struct {
	store    *AlertStore
	db       *storage.DB
	port     int
	apiKey   string
	alertsCh chan Alert

	mu          sync.RWMutex
	processes   []procmon.ProcessInfo
	connSummary map[string]int
	sseClients  int
}

func NewServer(port int, db *storage.DB) *Server {
	apiKey := os.Getenv("SENTINELMX_API_KEY")
	if apiKey == "" {
		apiKey = "changeme-set-SENTINELMX_API_KEY"
		log.Println("WARNING: SENTINELMX_API_KEY not set — using insecure default")
	}
	return &Server{
		store:       &AlertStore{},
		db:          db,
		port:        port,
		apiKey:      apiKey,
		alertsCh:    make(chan Alert, 50),
		connSummary: map[string]int{},
	}
}

func (s *Server) AddAlert(msg, severity string) {
	a := Alert{Message: msg, Timestamp: time.Now(), Severity: severity}
	s.store.Add(a)
	select {
	case s.alertsCh <- a:
	default:
	}
}

func (s *Server) UpdateProcesses(procs []procmon.ProcessInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes = procs
}

func (s *Server) UpdateConnections(summary map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connSummary = summary
}

// requireAPIKey valida el token en header X-API-Key o query param api_key
func (s *Server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if key != s.apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS restringido — solo el dashboard propio
		origin := r.Header.Get("Origin")
		allowed := map[string]bool{
			"http://localhost:3000":        true,
			"http://94.72.118.12:3000":     true,
		}
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		if r.Method == http.MethodOptions { w.WriteHeader(http.StatusOK); return }
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := telemetry.CollectMetrics()
	if err != nil { http.Error(w, err.Error(), 500); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.store.GetAll())
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.GetMetrics(120) // última hora (30s interval)
	if err != nil { http.Error(w, err.Error(), 500); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.processes)
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.connSummary)
}

const maxSSEClients = 20

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Rate limit: máximo 20 clientes SSE simultáneos
	s.mu.Lock()
	if s.sseClients >= maxSSEClients {
		s.mu.Unlock()
		http.Error(w, "too many clients", http.StatusTooManyRequests)
		return
	}
	s.sseClients++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.sseClients--
		s.mu.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok { http.Error(w, "streaming not supported", 500); return }
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C:
			m, err := telemetry.CollectMetrics()
			if err != nil { continue }
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
	// Todos los endpoints requieren API key
	mux.HandleFunc("/api/metrics",   s.requireAPIKey(s.handleMetrics))
	mux.HandleFunc("/api/alerts",    s.requireAPIKey(s.handleAlerts))
	mux.HandleFunc("/api/history",   s.requireAPIKey(s.handleHistory))
	mux.HandleFunc("/api/processes", s.requireAPIKey(s.handleProcesses))
	mux.HandleFunc("/api/network",   s.requireAPIKey(s.handleNetwork))
	mux.HandleFunc("/api/events",    s.requireAPIKey(s.handleEvents))
	// Health check público (sin datos sensibles)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":"0.2"}`)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      cors(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // 0 para SSE (streaming no tiene timeout)
		IdleTimeout:  60 * time.Second,
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
