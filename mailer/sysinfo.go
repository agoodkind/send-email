package mailer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SysInfo holds host metadata for email footers (Linux-oriented).
type SysInfo struct {
	Hostname       string
	UptimeHuman    string
	LoadAverage    string
	MemoryHuman    string
	DiskRootHuman  string
	PublicIPv4     string
	PublicIPv6     string
	ISP            string
	LocalIPv4Lines []string
	LocalIPv6Lines []string
}

// CollectSysInfo gathers system information. On non-Linux, some fields read as N/A.
func CollectSysInfo(ctx context.Context) SysInfo {
	h, _ := os.Hostname()
	si := SysInfo{Hostname: h}
	if runtime.GOOS != "linux" {
		si.UptimeHuman = "N/A (non-Linux)"
		si.LoadAverage = "N/A"
		si.MemoryHuman = "N/A"
		si.DiskRootHuman = "N/A"
		si.PublicIPv4 = racePublicIP(ctx, "tcp4")
		si.PublicIPv6 = racePublicIP(ctx, "tcp6")
		si.ISP = raceISP(ctx)
		si.LocalIPv4Lines, si.LocalIPv6Lines = localAddrs()
		return si
	}
	si.UptimeHuman = linuxUptimeString()
	si.LoadAverage = linuxLoadAvg()
	si.MemoryHuman = linuxMemHuman()
	si.DiskRootHuman = linuxDiskRoot()
	si.PublicIPv4 = racePublicIP(ctx, "tcp4")
	si.PublicIPv6 = racePublicIP(ctx, "tcp6")
	si.ISP = raceISP(ctx)
	si.LocalIPv4Lines, si.LocalIPv6Lines = localAddrs()
	return si
}

func linuxUptimeString() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "N/A"
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return "N/A"
	}
	var sec float64
	_, err = fmt.Sscanf(fields[0], "%f", &sec)
	if err != nil {
		return "N/A"
	}
	s := int64(sec)
	d := s / 86400
	s %= 86400
	h := s / 3600
	s %= 3600
	m := s / 60
	if d > 0 {
		return fmt.Sprintf("%d days, %d hours, %d minutes", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%d hours, %d minutes", h, m)
	}
	return fmt.Sprintf("%d minutes", m)
}

func linuxLoadAvg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "N/A"
	}
	parts := strings.Fields(string(data))
	if len(parts) < 3 {
		return strings.TrimSpace(string(data))
	}
	return strings.Join(parts[0:3], " ")
}

func linuxMemHuman() string {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return "N/A"
	}
	defer func() { _ = f.Close() }()
	var totalKB, availKB int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			_, _ = fmt.Sscanf(line, "MemTotal: %d kB", &totalKB)
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			_, _ = fmt.Sscanf(line, "MemAvailable: %d kB", &availKB)
		}
	}
	if totalKB == 0 {
		return "N/A"
	}
	usedKB := totalKB - availKB
	if usedKB < 0 {
		usedKB = 0
	}
	return fmt.Sprintf("%.1fG/%.1fG",
		float64(usedKB)/1024/1024,
		float64(totalKB)/1024/1024)
}

func linuxDiskRoot() string {
	var st syscall.Statfs_t
	err := syscall.Statfs("/", &st)
	if err != nil {
		return "N/A"
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	used := total - free
	pct := float64(0)
	if total > 0 {
		pct = float64(used) * 100 / float64(total)
	}
	return fmt.Sprintf("%.1fGiB/%.1fGiB (%.0f%%)",
		float64(used)/(1<<30), float64(total)/(1<<30), pct)
}

func racePublicIP(ctx context.Context, network string) string {
	urls := []string{
		"https://ifconfig.co/ip",
		"https://icanhazip.com",
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
	}
	return firstHTTPBody(ctx, network, urls, 5*time.Second)
}

func raceISP(ctx context.Context) string {
	urls := []string{
		"https://ifconfig.co/asn-org",
		"https://ipinfo.io/org",
		"http://ip-api.com/line/?fields=org",
	}
	return firstHTTPBody(ctx, "tcp", urls, 5*time.Second)
}

func firstHTTPBody(ctx context.Context, network string, urls []string, timeout time.Duration) string {
	parent, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var winner string
	client := &http.Client{Timeout: timeout}
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, n, addr string) (net.Conn, error) {
			if network == "tcp4" {
				return dialer.DialContext(ctx, "tcp4", addr)
			}
			if network == "tcp6" {
				return dialer.DialContext(ctx, "tcp6", addr)
			}
			return dialer.DialContext(ctx, "tcp", addr)
		},
	}
	client.Transport = tr

	for _, u := range urls {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			if winner != "" {
				return
			}
			req, err := http.NewRequestWithContext(parent, http.MethodGet, u, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return
			}
			b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
			if err != nil || len(b) == 0 {
				return
			}
			s := strings.TrimSpace(string(b))
			if s == "" {
				return
			}
			mu.Lock()
			if winner == "" {
				winner = s
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if winner != "" {
		return winner
	}
	return "N/A"
}

func localAddrs() (v4lines, v6lines []string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			s := ip.String()
			if ip.To4() != nil {
				v4lines = append(v4lines, fmt.Sprintf("%s %s", iface.Name, s))
			} else {
				v6lines = append(v6lines, fmt.Sprintf("%s %s", iface.Name, s))
			}
		}
	}
	return v4lines, v6lines
}
