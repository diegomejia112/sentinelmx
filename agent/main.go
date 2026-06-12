package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diegomejia11/sentinelmx/agent/internal/server"
	"github.com/diegomejia11/sentinelmx/agent/internal/telemetry"
)

func main() {
	fmt.Println("SentinelMX v0.1 — iniciando...")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := server.NewServer(8080)

	// Monitor conectado al servidor — alertas van directo al dashboard
	go telemetry.StartMonitor(ctx, 5*time.Second, func(alert string) {
		fmt.Printf("[%s] ALERTA: %s\n", time.Now().Format("15:04:05"), alert)
		srv.AddAlert(alert, "warning")
	})

	fmt.Println("Dashboard: http://localhost:3000")
	fmt.Println("API:       http://localhost:8080/api/metrics")

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	fmt.Println("SentinelMX detenido.")
}
