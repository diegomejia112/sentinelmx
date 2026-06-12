#!/bin/bash
set -e

# SentinelMX — Instalador de 1 comando
# Uso: curl -sSL https://raw.githubusercontent.com/diegomejia11/sentinelmx/main/scripts/install.sh | bash

REPO="https://github.com/diegomejia11/sentinelmx"
INSTALL_DIR="/opt/sentinelmx"
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

banner() {
  echo -e "${BOLD}"
  echo "  ███████╗███████╗███╗   ██╗████████╗██╗███╗   ██╗███████╗██╗     "
  echo "  ██╔════╝██╔════╝████╗  ██║╚══██╔══╝██║████╗  ██║██╔════╝██║     "
  echo "  ███████╗█████╗  ██╔██╗ ██║   ██║   ██║██╔██╗ ██║█████╗  ██║     "
  echo "  ╚════██║██╔══╝  ██║╚██╗██║   ██║   ██║██║╚██╗██║██╔══╝  ██║     "
  echo "  ███████║███████╗██║ ╚████║   ██║   ██║██║ ╚████║███████╗███████╗"
  echo "  ╚══════╝╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚═╝╚═╝  ╚═══╝╚══════╝╚══════╝"
  echo -e "${NC}"
  echo -e "  ${GREEN}Linux Security Monitor — Blue Team Edition${NC}"
  echo ""
}

check_root() {
  if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}Error: ejecuta como root (sudo bash install.sh)${NC}"
    exit 1
  fi
}

check_os() {
  if [[ ! -f /proc/version ]]; then
    echo -e "${RED}Error: SentinelMX requiere Linux con /proc${NC}"
    exit 1
  fi
  echo -e "${GREEN}✓ Linux detectado${NC}"
}

install_docker() {
  if command -v docker &>/dev/null; then
    echo -e "${GREEN}✓ Docker ya instalado$(docker --version | awk '{print $3}' | tr -d ',')${NC}"
    return
  fi
  echo -e "${YELLOW}→ Instalando Docker...${NC}"
  curl -fsSL https://get.docker.com | sh
  systemctl enable docker
  systemctl start docker
  echo -e "${GREEN}✓ Docker instalado${NC}"
}

install_sentinelmx() {
  echo -e "${YELLOW}→ Descargando SentinelMX...${NC}"
  mkdir -p "$INSTALL_DIR"

  if command -v git &>/dev/null; then
    git clone --depth=1 "$REPO" "$INSTALL_DIR/src" 2>/dev/null || \
      (cd "$INSTALL_DIR/src" && git pull)
  else
    echo -e "${RED}Error: git no instalado. Instala con: apt install git${NC}"
    exit 1
  fi

  echo -e "${YELLOW}→ Construyendo imágenes Docker...${NC}"
  cd "$INSTALL_DIR/src"
  docker build -f docker/Dockerfile.agent -t sentinelmx-agent:latest .
  echo -e "${GREEN}✓ Imagen del agente construida${NC}"
}

start_services() {
  echo -e "${YELLOW}→ Iniciando servicios...${NC}"
  cd "$INSTALL_DIR/src"
  docker compose -f docker/docker-compose.prod.yml up -d agent
  echo -e "${GREEN}✓ SentinelMX corriendo${NC}"
}

print_success() {
  echo ""
  echo -e "${GREEN}${BOLD}╔══════════════════════════════════════╗${NC}"
  echo -e "${GREEN}${BOLD}║     SentinelMX instalado con éxito   ║${NC}"
  echo -e "${GREEN}${BOLD}╚══════════════════════════════════════╝${NC}"
  echo ""
  echo -e "  API:       ${BOLD}http://$(hostname -I | awk '{print $1}'):8080/api/metrics${NC}"
  echo -e "  Dashboard: ${BOLD}http://$(hostname -I | awk '{print $1}'):3000${NC}"
  echo ""
  echo -e "  Ver logs:   ${YELLOW}docker logs -f sentinelmx-agent${NC}"
  echo -e "  Detener:    ${YELLOW}docker stop sentinelmx-agent${NC}"
  echo ""
}

banner
check_root
check_os
install_docker
install_sentinelmx
start_services
print_success
