#!/bin/bash
set -e

# Deploy SentinelMX al VPS de Cedecopy
# IMPORTANTE: corre en una red Docker separada — no toca Cedecopy
VPS="root@94.72.118.12"
REMOTE_DIR="/opt/sentinelmx"

echo "→ Sincronizando código al VPS..."
rsync -avz --exclude='node_modules/' --exclude='.next/' --exclude='dashboard/' \
  /home/diegomejia11/Documents/SentinelMX/ \
  "$VPS:$REMOTE_DIR/"

echo "→ Construyendo y levantando en el VPS..."
ssh "$VPS" "
  cd $REMOTE_DIR

  # Instalar Go si no existe
  if ! command -v go &>/dev/null; then
    apt-get update -q && apt-get install -y golang-go
  fi

  # Construir binario directamente (sin Docker si no está disponible)
  go build -o sentinelmx-agent ./agent/...

  # Crear servicio systemd para que arranque automático
  cat > /etc/systemd/system/sentinelmx.service << 'EOF'
[Unit]
Description=SentinelMX Security Agent
After=network.target

[Service]
ExecStart=$REMOTE_DIR/sentinelmx-agent
Restart=always
RestartSec=5
Environment=MONITOR_INTERVAL=5s

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable sentinelmx
  systemctl restart sentinelmx
  systemctl status sentinelmx --no-pager
"

echo ""
echo "✅ SentinelMX corriendo en el VPS"
echo "   API: http://94.72.118.12:8080/api/metrics"
echo "   Logs: ssh $VPS 'journalctl -u sentinelmx -f'"
