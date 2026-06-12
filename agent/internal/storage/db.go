package storage

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type MetricRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	CPU        float64   `json:"cpu"`
	RAM        float64   `json:"ram"`
	SwapKB     uint64    `json:"swap_kb"`
	WriteCount uint64    `json:"write_count"`
}

type AlertRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
}

type dbFile struct {
	Metrics []MetricRecord `json:"metrics"`
	Alerts  []AlertRecord  `json:"alerts"`
}

type DB struct {
	path string
	mu   sync.Mutex
}

func NewDB(path string) (*DB, error) {
	db := &DB{path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		empty := dbFile{Metrics: []MetricRecord{}, Alerts: []AlertRecord{}}
		data, _ := json.MarshalIndent(empty, "", "  ")
		if err := os.WriteFile(path, data, 0600); err != nil {
			return nil, err
		}
	}
	return db, nil
}

func (db *DB) readFile() (*dbFile, error) {
	data, err := os.ReadFile(db.path)
	if err != nil {
		return nil, err
	}
	var f dbFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Metrics == nil { f.Metrics = []MetricRecord{} }
	if f.Alerts == nil  { f.Alerts = []AlertRecord{} }
	return &f, nil
}

func (db *DB) writeFile(f *dbFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil { return err }
	return os.WriteFile(db.path, data, 0600)
}

func (db *DB) SaveMetric(m MetricRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	f, err := db.readFile()
	if err != nil { return err }
	f.Metrics = append(f.Metrics, m)
	if len(f.Metrics) > 2880 {
		f.Metrics = f.Metrics[len(f.Metrics)-2880:]
	}
	return db.writeFile(f)
}

func (db *DB) GetMetrics(last int) ([]MetricRecord, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	f, err := db.readFile()
	if err != nil { return nil, err }
	if last <= 0 || last >= len(f.Metrics) { return f.Metrics, nil }
	return f.Metrics[len(f.Metrics)-last:], nil
}

func (db *DB) SaveAlert(a AlertRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	f, err := db.readFile()
	if err != nil { return err }
	f.Alerts = append(f.Alerts, a)
	if len(f.Alerts) > 500 {
		f.Alerts = f.Alerts[len(f.Alerts)-500:]
	}
	return db.writeFile(f)
}

func (db *DB) GetAlerts(last int) ([]AlertRecord, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	f, err := db.readFile()
	if err != nil { return nil, err }
	if last <= 0 || last >= len(f.Alerts) { return f.Alerts, nil }
	return f.Alerts[len(f.Alerts)-last:], nil
}
