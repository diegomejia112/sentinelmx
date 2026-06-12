package telemetry

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SystemMetrics struct {
	MemUsedPercent  float64
	CPUUsagePercent float64
	SwapUsedKB      uint64
	WriteCount      uint64
	OpenatCount     uint64
	Timestamp       time.Time
}

func readMemInfo() (totalKB, usedKB, swapUsedKB uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	var memTotal, memFree, buffers, cached, sreclaimable, swapTotal, swapFree uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = parseUint64Field(line)
		case strings.HasPrefix(line, "MemFree:"):
			memFree = parseUint64Field(line)
		case strings.HasPrefix(line, "Buffers:"):
			buffers = parseUint64Field(line)
		case strings.HasPrefix(line, "Cached:"):
			cached = parseUint64Field(line)
		case strings.HasPrefix(line, "SReclaimable:"):
			sreclaimable = parseUint64Field(line)
		case strings.HasPrefix(line, "SwapTotal:"):
			swapTotal = parseUint64Field(line)
		case strings.HasPrefix(line, "SwapFree:"):
			swapFree = parseUint64Field(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, 0, fmt.Errorf("scan /proc/meminfo: %w", err)
	}
	if memTotal == 0 {
		return 0, 0, 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}

	usableFree := memFree + buffers + cached + sreclaimable
	if usableFree > memTotal {
		usableFree = memTotal
	}
	if swapTotal > 0 {
		swapUsedKB = swapTotal - swapFree
	}
	return memTotal, memTotal - usableFree, swapUsedKB, nil
}

func parseUint64Field(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	val, _ := strconv.ParseUint(fields[1], 10, 64)
	return val
}

func readCPUStat() (total, idle uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, fmt.Errorf("open /proc/stat: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			return 0, 0, fmt.Errorf("unexpected cpu line: %s", line)
		}
		var ticks [8]uint64
		for i := 0; i < 8; i++ {
			ticks[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
			total += ticks[i]
		}
		idle = ticks[3]
		break
	}
	return total, idle, scanner.Err()
}

func readWriteCount() (uint64, error) {
	data, err := os.ReadFile("/proc/self/io")
	if err != nil {
		return 0, fmt.Errorf("read /proc/self/io: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "syscw:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				return val, nil
			}
		}
	}
	return 0, fmt.Errorf("syscw not found")
}

func readOpenatCount() (uint64, error) {
	fdDir, err := os.Open("/proc/self/fd")
	if err != nil {
		return 0, fmt.Errorf("open /proc/self/fd: %w", err)
	}
	defer fdDir.Close()
	names, err := fdDir.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	return uint64(len(names)), nil
}

func CollectMetrics() (*SystemMetrics, error) {
	totalKB, usedKB, swapUsedKB, err := readMemInfo()
	if err != nil {
		return nil, err
	}

	cpuTotal1, cpuIdle1, err := readCPUStat()
	if err != nil {
		return nil, err
	}
	time.Sleep(200 * time.Millisecond)
	cpuTotal2, cpuIdle2, err := readCPUStat()
	if err != nil {
		return nil, err
	}

	var cpuPercent float64
	if delta := cpuTotal2 - cpuTotal1; delta > 0 {
		cpuPercent = (1.0 - float64(cpuIdle2-cpuIdle1)/float64(delta)) * 100.0
		if cpuPercent < 0 {
			cpuPercent = 0
		}
		if cpuPercent > 100 {
			cpuPercent = 100
		}
	}

	writeCount, _ := readWriteCount()
	openatCount, _ := readOpenatCount()

	memPercent := 0.0
	if totalKB > 0 {
		memPercent = math.Round(float64(usedKB)/float64(totalKB)*10000) / 100
	}

	return &SystemMetrics{
		MemUsedPercent:  memPercent,
		CPUUsagePercent: cpuPercent,
		SwapUsedKB:      swapUsedKB,
		WriteCount:      writeCount,
		OpenatCount:     openatCount,
		Timestamp:       time.Now(),
	}, nil
}

func DetectAnomaly(current, previous *SystemMetrics) (bool, string) {
	if current == nil || previous == nil {
		return false, ""
	}
	var reasons []string

	if current.CPUUsagePercent > 90.0 {
		reasons = append(reasons, fmt.Sprintf("CPU %.1f%% > 90%%", current.CPUUsagePercent))
	}
	if current.MemUsedPercent > 95.0 {
		reasons = append(reasons, fmt.Sprintf("RAM %.1f%% > 95%%", current.MemUsedPercent))
	}
	if current.SwapUsedKB > 0 {
		reasons = append(reasons, fmt.Sprintf("Swap activo: %d KB", current.SwapUsedKB))
	}
	if secs := current.Timestamp.Sub(previous.Timestamp).Seconds(); secs > 0 {
		if delta := current.WriteCount - previous.WriteCount; delta > 500 {
			reasons = append(reasons, fmt.Sprintf("Escrituras +%d/s (posible ransomware)", delta))
		}
	}

	if len(reasons) > 0 {
		return true, strings.Join(reasons, " | ")
	}
	return false, ""
}

func StartMonitor(ctx context.Context, interval time.Duration, alertFn func(string)) {
	if alertFn == nil {
		alertFn = func(s string) { fmt.Println("[ALERTA]", s) }
	}

	var (
		prev *SystemMetrics
		mu   sync.Mutex
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m, err := CollectMetrics()
			if err != nil {
				alertFn(fmt.Sprintf("Error recolectando métricas: %v", err))
				continue
			}
			mu.Lock()
			if prev != nil {
				if anomaly, desc := DetectAnomaly(m, prev); anomaly {
					alertFn(desc)
				}
			}
			prev = m
			mu.Unlock()
		}
	}
}
