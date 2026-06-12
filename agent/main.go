package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diegomejia11/sentinelmx/agent/internal/netmon"
	"github.com/diegomejia11/sentinelmx/agent/internal/procmon"
	"github.com/diegomejia11/sentinelmx/agent/internal/server"
	"github.com/diegomejia11/sentinelmx/agent/internal/sshmon"
	"github.com/diegomejia11/sentinelmx/agent/internal/storage"
	"github.com/diegomejia11/sentinelmx/agent/internal/telemetry"
)

func main() {
	fmt.Println("SentinelMX v0.2 — iniciando...")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := storage.NewDB("/var/lib/sentinelmx/data.json")
	if err != nil {
		// fallback a directorio local
		os.MkdirAll("./data", 0755)
		db, err = storage.NewDB("./data/sentinelmx.json")
		if err != nil {
			fmt.Fprintf(os.Stderr, "DB error: %v\n", err)
			os.Exit(1)
		}
	}

	srv := server.NewServer(8080, db)

	// Monitor de telemetría (/proc/meminfo + /proc/stat)
	go telemetry.StartMonitor(ctx, 5*time.Second, func(alert string) {
		fmt.Printf("[%s] ALERTA SISTEMA: %s\n", time.Now().Format("15:04:05"), alert)
		srv.AddAlert(alert, "warning")
		db.SaveAlert(storage.AlertRecord{Timestamp: time.Now(), Message: alert, Severity: "warning"})
	})

	// Monitor de procesos (cada 15s)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done(): return
			case <-ticker.C:
				procs, err := procmon.ScanProcesses()
				if err != nil { continue }
				for _, alert := range procmon.DetectSuspicious(procs) {
					fmt.Printf("[%s] ALERTA PROCESO: %s\n", time.Now().Format("15:04:05"), alert)
					srv.AddAlert(alert, "critical")
					db.SaveAlert(storage.AlertRecord{Timestamp: time.Now(), Message: alert, Severity: "critical"})
				}
				srv.UpdateProcesses(procmon.TopByMemory(procs, 10))
			}
		}
	}()

	// Monitor de red (/proc/net/tcp)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done(): return
			case <-ticker.C:
				conns, err := netmon.ScanConnections()
				if err != nil { continue }
				for _, alert := range netmon.DetectSuspicious(conns) {
					fmt.Printf("[%s] ALERTA RED: %s\n", time.Now().Format("15:04:05"), alert)
					srv.AddAlert(alert, "warning")
					db.SaveAlert(storage.AlertRecord{Timestamp: time.Now(), Message: "[NET] " + alert, Severity: "warning"})
				}
				srv.UpdateConnections(netmon.Summary(conns))
			}
		}
	}()

	// Monitor SSH (auth.log)
	go func() {
		authLog := "/var/log/auth.log"
		if _, err := os.Stat(authLog); os.IsNotExist(err) {
			authLog = "/var/log/secure" // Fedora/RHEL
		}
		sshCh := make(chan sshmon.SSHEvent, 50)
		go sshmon.TailAuthLog(ctx, authLog, sshCh)
		for {
			select {
			case <-ctx.Done(): return
			case ev := <-sshCh:
				var msg, severity string
				if ev.Type == "failed_login" || ev.Type == "invalid_user" {
					msg = fmt.Sprintf("[SSH] Failed login: user=%s from=%s", ev.User, ev.IP)
					severity = "warning"
				} else {
					msg = fmt.Sprintf("[SSH] Login exitoso: user=%s from=%s", ev.User, ev.IP)
					severity = "info"
				}
				fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), msg)
				srv.AddAlert(msg, severity)
				db.SaveAlert(storage.AlertRecord{Timestamp: time.Now(), Message: msg, Severity: severity})
			}
		}
	}()

	// Guardar métricas en DB cada 30s
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done(): return
			case <-ticker.C:
				m, err := telemetry.CollectMetrics()
				if err != nil { continue }
				db.SaveMetric(storage.MetricRecord{
					Timestamp:  m.Timestamp,
					CPU:        m.CPUUsagePercent,
					RAM:        m.MemUsedPercent,
					SwapKB:     m.SwapUsedKB,
					WriteCount: m.WriteCount,
				})
			}
		}
	}()

	fmt.Printf("Dashboard: http://localhost:3000\nAPI:       http://localhost:8080/api/metrics\n")

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
	}
	fmt.Println("SentinelMX detenido.")
}
