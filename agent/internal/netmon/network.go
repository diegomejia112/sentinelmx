package netmon

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)


type Connection struct {
	LocalAddr  string `json:"local"`
	RemoteAddr string `json:"remote"`
	State      string `json:"state"`
}

var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
	"04": "FIN_WAIT1",   "05": "FIN_WAIT2","06": "TIME_WAIT",
	"07": "CLOSE",       "08": "CLOSE_WAIT","09": "LAST_ACK",
	"0A": "LISTEN",      "0B": "CLOSING",
}

func parseHexAddr(hex string) string {
	parts := strings.Split(hex, ":")
	if len(parts) != 2 { return "" }
	port, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil { return "" }
	ipHex := parts[0]
	if len(ipHex) == 8 {
		b := make([]byte, 4)
		for i := 0; i < 4; i++ {
			v, err := strconv.ParseUint(ipHex[i*2:i*2+2], 16, 8)
			if err != nil { return "" }
			b[3-i] = byte(v)
		}
		return fmt.Sprintf("%s:%d", net.IPv4(b[0], b[1], b[2], b[3]).String(), port)
	}
	return fmt.Sprintf("IPv6:%d", port)
}

func parseProcNetTCP(path string) ([]Connection, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	var conns []Connection
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 { continue }
		remHex := fields[2]
		if strings.HasPrefix(remHex, "00000000:") || remHex == "00000000:0000" { continue }
		state := tcpStates[fields[3]]
		if state == "" { state = "UNKNOWN" }
		conns = append(conns, Connection{
			LocalAddr:  parseHexAddr(fields[1]),
			RemoteAddr: parseHexAddr(remHex),
			State:      state,
		})
	}
	return conns, scanner.Err()
}

func ScanConnections() ([]Connection, error) {
	conns, _ := parseProcNetTCP("/proc/net/tcp")
	v6, _ := parseProcNetTCP("/proc/net/tcp6")
	return append(conns, v6...), nil
}

func isPrivateIP(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil { host = addr }
	ip := net.ParseIP(host)
	if ip == nil { return false }
	if ip.IsLoopback() { return true }
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return false
}

var knownPorts = map[int]bool{80: true, 443: true, 22: true, 3306: true, 3000: true, 3001: true, 8080: true, 5432: true}

// whitelistedIPs son IPs de administración propias — no generar alertas
var whitelistedIPs = map[string]bool{}

func init() {
	// Cargar whitelist desde variable de entorno SENTINELMX_WHITELIST_IPS
	// Formato: "1.2.3.4,5.6.7.8"
	wl := os.Getenv("SENTINELMX_WHITELIST_IPS")
	if wl != "" {
		for _, ip := range strings.Split(wl, ",") {
			whitelistedIPs[strings.TrimSpace(ip)] = true
		}
	}
}

func DetectSuspicious(conns []Connection) []string {
	var alerts []string
	established := 0
	ipCount := map[string]int{}
	for _, c := range conns {
		if c.State != "ESTABLISHED" { continue }
		established++
		host, _, _ := net.SplitHostPort(c.RemoteAddr)
		ipCount[host]++
		if !isPrivateIP(c.RemoteAddr) && !whitelistedIPs[host] {
			_, portStr, err := net.SplitHostPort(c.RemoteAddr)
			if err == nil {
				port, _ := strconv.Atoi(portStr)
				// Ignorar conexiones SSH entrantes vistas como salientes (bug conocido /proc/net/tcp)
				if !knownPorts[port] && port != 22 {
					alerts = append(alerts, fmt.Sprintf("Suspicious outbound: %s → port %d", host, port))
				}
			}
		}
	}
	if established > 50 {
		alerts = append(alerts, fmt.Sprintf("High connections: %d established (possible DDoS)", established))
	}
	for ip, count := range ipCount {
		if count > 20 {
			alerts = append(alerts, fmt.Sprintf("IP %s has %d connections", ip, count))
		}
	}
	return alerts
}

func Summary(conns []Connection) map[string]int {
	result := map[string]int{}
	for _, c := range conns { result[c.State]++ }
	return result
}
