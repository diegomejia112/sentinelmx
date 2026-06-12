package sshmon

import (
	"bufio"
	"context"
	"os"
	"strings"
	"time"
)

type SSHEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	User      string    `json:"user"`
	IP        string    `json:"ip"`
	Success   bool      `json:"success"`
}

func parseSSHLine(line string) *SSHEvent {
	if !strings.Contains(line, "sshd") { return nil }
	lower := strings.ToLower(line)
	fields := strings.Fields(line)

	ev := &SSHEvent{Timestamp: time.Now()}

	switch {
	case strings.Contains(lower, "failed password"):
		ev.Type = "failed_login"
		ev.Success = false
	case strings.Contains(lower, "accepted password") || strings.Contains(lower, "accepted publickey"):
		ev.Type = "successful_login"
		ev.Success = true
	case strings.Contains(lower, "invalid user"):
		ev.Type = "invalid_user"
		ev.Success = false
	default:
		return nil
	}

	// Extraer IP y usuario
	for i, f := range fields {
		if f == "from" && i+1 < len(fields) {
			ev.IP = strings.Split(fields[i+1], " ")[0]
		}
		if f == "for" && i+1 < len(fields) && fields[i+1] != "invalid" {
			ev.User = fields[i+1]
		}
		if f == "user" && i+1 < len(fields) {
			ev.User = fields[i+1]
		}
	}
	return ev
}

func ParseAuthLog(path string) ([]SSHEvent, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	var events []SSHEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if ev := parseSSHLine(scanner.Text()); ev != nil {
			events = append(events, *ev)
		}
	}
	return events, scanner.Err()
}

func TailAuthLog(ctx context.Context, path string, eventCh chan<- SSHEvent) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastSize := int64(0)
	if stat, err := os.Stat(path); err == nil { lastSize = stat.Size() }

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stat, err := os.Stat(path)
			if err != nil || stat.Size() <= lastSize { continue }
			f, err := os.Open(path)
			if err != nil { continue }
			f.Seek(lastSize, 0)
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				if ev := parseSSHLine(scanner.Text()); ev != nil {
					select {
					case eventCh <- *ev:
					case <-ctx.Done():
						f.Close(); return
					}
				}
			}
			lastSize = stat.Size()
			f.Close()
		}
	}
}

func DetectBruteForce(events []SSHEvent, windowSecs int) map[string]int {
	cutoff := time.Now().Add(-time.Duration(windowSecs) * time.Second)
	counts := map[string]int{}
	for _, ev := range events {
		if !ev.Success && ev.Timestamp.After(cutoff) && ev.IP != "" {
			counts[ev.IP]++
		}
	}
	result := map[string]int{}
	for ip, n := range counts {
		if n >= 5 { result[ip] = n }
	}
	return result
}
