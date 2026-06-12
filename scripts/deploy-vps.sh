#!/bin/bash
set -e

VPS="root@94.72.118.12"
REMOTE_DIR="/opt/sentinelmx"

echo "→ Sincronizando código al VPS..."
rsync -avz --exclude='node_modules/' --exclude='.next/' --exclude='dashboard/' \
  /home/diegomejia11/Documents/SentinelMX/ \
  "$VPS:$REMOTE_DIR/"

echo "→ Construyendo y configurando en el VPS..."
ssh "$VPS" "
  cd $REMOTE_DIR

  if ! command -v go &>/dev/null; then
    apt-get update -q && apt-get install -y golang-go
  fi

  go build -buildvcs=false -o sentinelmx-agent ./agent/main.go

  # Crear usuario dedicado sin shell ni home (principio de mínimo privilegio)
  if ! id sentinel &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin sentinel
    echo '[+] Usuario sentinel creado'
  fi

  # Directorios con permisos correctos
  mkdir -p /var/lib/sentinelmx
  chown sentinel:sentinel /var/lib/sentinelmx
  chmod 700 /var/lib/sentinelmx
  chown root:root /opt/sentinelmx/sentinelmx-agent
  chmod 755 /opt/sentinelmx/sentinelmx-agent

  # Generar API key si no existe
  if [ ! -f /etc/sentinelmx/api.env ]; then
    mkdir -p /etc/sentinelmx
    chmod 700 /etc/sentinelmx
    API_KEY=\$(openssl rand -hex 32)
    echo \"SENTINELMX_API_KEY=\$API_KEY\" > /etc/sentinelmx/api.env
    chmod 600 /etc/sentinelmx/api.env
    echo \"[+] API key generada: \$API_KEY\"
    echo '    Guárdala — la necesitas para el dashboard'
  fi

  # Servicio systemd hardened (sin root)
  cat > /etc/systemd/system/sentinelmx.service << 'EOF'
[Unit]
Description=SentinelMX Security Agent
After=network.target

[Service]
Type=simple
User=sentinel
Group=sentinel
ExecStart=/opt/sentinelmx/sentinelmx-agent
Restart=always
RestartSec=5
EnvironmentFile=/etc/sentinelmx/api.env
Environment=MONITOR_INTERVAL=5s

# Hardening — mínimo privilegio
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/sentinelmx
ReadOnlyPaths=/proc
PrivateTmp=yes
CapabilityBoundingSet=

[Install]
WantedBy=multi-user.target
EOF

  # auth.log necesita permisos para el usuario sentinel
  if [ -f /var/log/auth.log ]; then
    setfacl -m u:sentinel:r /var/log/auth.log 2>/dev/null || \
      chmod o+r /var/log/auth.log
  fi

  systemctl daemon-reload
  systemctl enable sentinelmx
  systemctl restart sentinelmx
  sleep 2
  systemctl status sentinelmx --no-pager | head -12
"

echo ""
echo "✅ SentinelMX desplegado de forma segura"
echo "   API: http://94.72.118.12:8080/health (público)"
echo "   Métricas: requieren X-API-Key header"
