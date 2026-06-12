package procmon

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ProcessInfo struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	MemRSSKB uint64 `json:"mem_rss_kb"`
	State    string `json:"state"`
	Hidden   bool   `json:"hidden"`
}

func ScanProcesses() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("cannot read /proc: %w", err)
	}
	var procs []ProcessInfo
	for _, e := range entries {
		if !e.IsDir() { continue }
		pid, err := strconv.Atoi(e.Name())
		if err != nil { continue }
		info, err := parseStatus(filepath.Join("/proc", e.Name(), "status"), pid)
		if err != nil { continue }
		procs = append(procs, *info)
	}
	return procs, nil
}

func parseStatus(path string, pid int) (*ProcessInfo, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	info := &ProcessInfo{PID: pid}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Name:"):
			info.Name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			if info.Name == "" || info.Name == "?" { info.Hidden = true }
		case strings.HasPrefix(line, "VmRSS:"):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.ParseUint(parts[1], 10, 64)
				info.MemRSSKB = val
			}
		case strings.HasPrefix(line, "State:"):
			parts := strings.Fields(line)
			if len(parts) >= 2 { info.State = parts[1] }
		}
	}
	return info, scanner.Err()
}

func DetectSuspicious(procs []ProcessInfo) []string {
	var alerts []string
	for _, p := range procs {
		if p.Hidden {
			alerts = append(alerts, fmt.Sprintf("Hidden process detected PID %d", p.PID))
		}
		if p.MemRSSKB > 1048576 { // 1GB — evitar falsos positivos con Java/Docker en producción
			alerts = append(alerts, fmt.Sprintf("High memory process: %s (%.0f MB)", p.Name, float64(p.MemRSSKB)/1024))
		}
	}
	if len(procs) > 300 {
		alerts = append(alerts, fmt.Sprintf("Abnormal process count: %d", len(procs)))
	}
	return alerts
}

func TopByMemory(procs []ProcessInfo, n int) []ProcessInfo {
	sorted := make([]ProcessInfo, len(procs))
	copy(sorted, procs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MemRSSKB > sorted[j].MemRSSKB })
	if n > len(sorted) { n = len(sorted) }
	return sorted[:n]
}
