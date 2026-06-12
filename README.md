# SentinelMX

**Lightweight Linux security monitor written in Go. Real-time anomaly detection with zero external dependencies.**

```
curl -sSL https://raw.githubusercontent.com/diegomejia11/sentinelmx/main/scripts/install.sh | sudo bash
```

---

## What it does

SentinelMX runs as a lightweight Go agent on any Linux server. It reads `/proc` directly — no agents, no cloud, no telemetry sent anywhere. When something suspicious happens, it alerts you instantly through a web dashboard.

```
Your Linux Server
      │
   Go Agent          reads /proc every 5s
      │              detects anomalies
      ▼
  HTTP + SSE    ──►  Web Dashboard (real-time)
```

## Detection capabilities

| Threat | How it detects |
|---|---|
| Ransomware | Write syscall spike > 500/s over baseline |
| High CPU anomaly | Usage > 90% sustained |
| Memory pressure | RAM > 95% used |
| Swap key risk | Any swap usage while cryptographic keys are in memory |
| Custom thresholds | Configurable via environment variables |

## Install in 60 seconds

```bash
# On any Linux server (requires Docker)
curl -sSL https://raw.githubusercontent.com/diegomejia11/sentinelmx/main/scripts/install.sh | sudo bash
```

Or run manually:

```bash
git clone https://github.com/diegomejia11/sentinelmx
cd sentinelmx
go run ./agent/...
```

Then open `http://localhost:3000`

## Dashboard

- Real-time CPU, RAM, Swap gauges
- Color-coded status: green / yellow / red
- Live alert feed with timestamps
- Auto-reconnects if connection drops

## Tech stack

- **Agent:** Go 1.23 — reads `/proc/meminfo`, `/proc/stat`, `/proc/self/io`
- **API:** Go stdlib HTTP server with Server-Sent Events (SSE)
- **Dashboard:** Next.js 15 with real-time updates
- **Deploy:** Docker, zero external dependencies

## Architecture

```
sentinelmx/
├── agent/
│   ├── main.go                  # entry point + signal handling
│   └── internal/
│       ├── telemetry/           # /proc reader + anomaly detection
│       └── server/              # HTTP API + SSE streaming
├── dashboard/                   # Next.js real-time UI
├── docker/                      # Dockerfiles + compose
└── scripts/
    └── install.sh               # 1-command installer
```

## Why SentinelMX

Most security monitoring tools cost $50k+/year (Splunk, Datadog, CrowdStrike). Small businesses running Linux servers have no affordable alternative. SentinelMX is open source, installs in 60 seconds, and uses < 0.1% CPU.

## Roadmap

- [x] `/proc` anomaly detection
- [x] Real-time web dashboard
- [x] Docker deployment
- [ ] Multi-server support
- [ ] Email + Slack alerts
- [ ] Compliance reports (SOC 2 / NIST)
- [ ] ML-based baseline learning

## License

MIT — free for personal and commercial use.

---

Built by [Diego Mejia Arroyo](https://github.com/diegomejia11) · Monterrey, Mexico
