// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SystemUptimeSeconds returns system uptime in seconds.
// On Linux reads /proc/uptime; on Darwin uses kern.boottime.
// Returns 0 on error or unsupported platform.
func SystemUptimeSeconds() float64 {
	switch runtime.GOOS {
	case "linux":
		return linuxUptime()
	case "darwin":
		return darwinUptime()
	default:
		return 0
	}
}

func linuxUptime() float64 {
	f, err := os.Open("/proc/uptime")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0
	}
	parts := strings.Fields(sc.Text())
	if len(parts) < 1 {
		return 0
	}
	secs, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	return secs
}

var boottimeSecRe = regexp.MustCompile(`sec\s*=\s*(\d+)`)

func darwinUptime() float64 {
	out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return 0
	}
	m := boottimeSecRe.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return 0
	}
	sec, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0
	}
	boot := time.Unix(sec, 0)
	return time.Since(boot).Seconds()
}
