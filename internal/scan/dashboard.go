// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"fmt"
	"io"
	"strings"

	"github.com/keldron-ai/keldron-agent/internal/api"
	"github.com/keldron-ai/keldron-agent/internal/health"
)

// ANSI color codes for dashboard
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
)

const dashboardWidth = 52

// ratingColor returns ANSI color for health/risk rating.
func ratingColor(rating string) string {
	switch strings.ToLower(rating) {
	case "healthy", "normal", "excellent", "stable":
		return colorGreen
	case "compressed", "slow", "elevated", "warning":
		return colorYellow
	case "critical", "poor", "unstable":
		return colorRed
	default:
		return colorDim
	}
}

// riskColor returns ANSI color for risk score using API thresholds.
func riskColor(score float64, warning, critical float64) string {
	if score >= critical {
		return colorRed
	}
	if score >= warning {
		return colorYellow
	}
	return colorGreen
}

// trendSymbol returns arrow for trend.
func trendSymbol(trend string) string {
	switch strings.ToLower(trend) {
	case "rising":
		return "▲"
	case "falling":
		return "▼"
	default:
		return "—"
	}
}

// RenderDashboard writes the single-device dashboard to w.
func RenderDashboard(w io.Writer, status *api.StatusResponse, risk *api.RiskResponse, opts RenderOpts) {
	if status == nil {
		return
	}
	d := status.Device
	t := status.Telemetry
	a := status.Agent
	r := status.Risk

	// Use risk response for sub-scores and thresholds if available
	var warning, critical float64 = 65, 82
	subScores := &api.SubScores{}
	if risk != nil {
		warning = risk.Thresholds.Warning
		critical = risk.Thresholds.Critical
		subScores = &risk.SubScores
		r = api.RiskSummary{
			CompositeScore: risk.Composite.Score,
			Severity:       risk.Composite.Severity,
			Trend:          risk.Composite.Trend,
			TrendDelta:     risk.Composite.TrendDelta,
		}
	}

	boxTop := "╔" + strings.Repeat("═", dashboardWidth-2) + "╗"
	boxMid := "╠" + strings.Repeat("═", dashboardWidth-2) + "╣"
	boxBot := "╚" + strings.Repeat("═", dashboardWidth-2) + "╝"
	boxLine := "║"

	padRight := func(s string, n int) string {
		runes := []rune(s)
		if len(runes) >= n {
			return string(runes[:n-1]) + "…"
		}
		return s + strings.Repeat(" ", n-len(runes))
	}

	contentWidth := dashboardWidth - 4 // "║ " + " ║"

	if !opts.Quiet {
		fmt.Fprintln(w, boxTop)
		hostname := d.Hostname
		if hostname == "" {
			hostname = "Unknown"
		}
		fmt.Fprintf(w, "%s  🖥️  %s%s\n", boxLine, padRight(hostname, contentWidth-4), boxLine)
		hw := d.Hardware
		if hw == "" {
			hw = "unknown"
		}
		adapter := d.Adapter
		if adapter == "" {
			adapter = "unknown"
		}
		header2 := fmt.Sprintf("%s · %s · %s · v%s", hw, d.OS, adapter, a.Version)
		fmt.Fprintf(w, "%s  %s%s\n", boxLine, padRight(header2, contentWidth), boxLine)
		fmt.Fprintln(w, boxMid)

		// TELEMETRY
		fmt.Fprintf(w, "%s  TELEMETRY%s\n", boxLine, padRight("", contentWidth-10)+boxLine)
		tempStr := "—"
		if t.TemperatureC > 0 {
			tempStr = fmt.Sprintf("%.1f°C", t.TemperatureC)
		}
		thermalState := t.ThermalState
		if thermalState == "" {
			thermalState = "nominal"
		}
		telLine := fmt.Sprintf("  🌡️ Temperature     %-8s  %s", tempStr, thermalState)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(telLine, contentWidth), boxLine)
		utilBar := utilizationBar(t.GPUUtilizationPct)
		utilLine := fmt.Sprintf("  ⚡ GPU Utilization  %s %5.1f%%", utilBar, t.GPUUtilizationPct)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(utilLine, contentWidth), boxLine)
		powerStr := "—"
		if t.PowerDrawW > 0 {
			powerStr = fmt.Sprintf("%.2fW", t.PowerDrawW)
		}
		powerLine := fmt.Sprintf("  🔌 Power Draw      %s", powerStr)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(powerLine, contentWidth), boxLine)
		memStr := "—"
		if t.MemoryTotalBytes > 0 {
			usedGB := float64(t.MemoryUsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(t.MemoryTotalBytes) / (1024 * 1024 * 1024)
			memStr = fmt.Sprintf("%.1f / %.1f GB   %5.1f%%", usedGB, totalGB, t.MemoryUsedPct)
		}
		memLine := fmt.Sprintf("  🧠 Memory          %s", memStr)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(memLine, contentWidth), boxLine)
		fmt.Fprintln(w, boxMid)

		// RISK ANALYSIS
		riskHeader := "  RISK ANALYSIS" + padRight("", contentWidth-28) + "Score  Wt"
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(riskHeader, contentWidth), boxLine)
		sevColor := riskColor(r.CompositeScore, warning, critical)
		sevDot := ratingColor(r.Severity)
		compStr := fmt.Sprintf("%.2f", r.CompositeScore)
		if r.CompositeScore >= 10 {
			compStr = fmt.Sprintf("%.1f", r.CompositeScore)
		}
		compLine := fmt.Sprintf("  🎯 Composite        %s%-6s%s  %s● %s%s", sevColor, compStr, colorReset, sevDot, r.Severity, colorReset)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(compLine, contentWidth), boxLine)
		thermLine := fmt.Sprintf("     Thermal          %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Thermal.Score), subScores.Thermal.Weight)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(thermLine, contentWidth), boxLine)
		powerRiskLine := fmt.Sprintf("     Power            %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Power.Score), subScores.Power.Weight)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(powerRiskLine, contentWidth), boxLine)
		volLine := fmt.Sprintf("     Volatility       %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Volatility.Score), subScores.Volatility.Weight)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(volLine, contentWidth), boxLine)
		corrLine := fmt.Sprintf("     Correlated       %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Correlated.Score), subScores.Correlated.Weight)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(corrLine, contentWidth), boxLine)
		trendStr := fmt.Sprintf("  Trend: %s (Δ %+.2f)", trendSymbol(r.Trend), r.TrendDelta)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(trendStr, contentWidth), boxLine)
		fmt.Fprintln(w, boxMid)

		// DEVICE HEALTH
		healthHeader := "  DEVICE HEALTH" + padRight("", contentWidth-16)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(healthHeader, contentWidth), boxLine)
		renderHealthSection(w, boxLine, contentWidth, status.Health, t.PowerDrawW)
		fmt.Fprintln(w, boxMid)

		// Footer
		uptimeStr := formatUptime(d.UptimeSeconds)
		pollStr := fmt.Sprintf("%ds", a.PollIntervalS)
		cloudStr := "✗"
		if a.CloudConnected {
			cloudStr = "✓"
		}
		footer := fmt.Sprintf("  Uptime: %s · Poll: %s · Cloud: %s", uptimeStr, pollStr, cloudStr)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(footer, contentWidth), boxLine)
		fmt.Fprintln(w, boxBot)
	}
}

func utilizationBar(pct float64) string {
	const blocks = 10
	filled := int(pct / 10)
	if filled > blocks {
		filled = blocks
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", blocks-filled)
}

func formatUptime(secs float64) string {
	if secs <= 0 {
		return "—"
	}
	d := int(secs) / 86400
	h := (int(secs) % 86400) / 3600
	if d > 0 {
		return fmt.Sprintf("%dd %dh", d, h)
	}
	m := (int(secs) % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func renderHealthSection(w io.Writer, boxLine string, contentWidth int, h *health.DeviceHealthSnapshot, powerW float64) {
	padRight := func(s string, n int) string {
		runes := []rune(s)
		if len(runes) >= n {
			return string(runes[:n-1]) + "…"
		}
		return s + strings.Repeat(" ", n-len(runes))
	}
	line := func(icon, left, right string) {
		content := fmt.Sprintf("  %s %-18s  %s", icon, left, right)
		fmt.Fprintf(w, "%s%s%s\n", boxLine, padRight(content, contentWidth), boxLine)
	}

	// TDR
	if h != nil && h.ThermalDynamicRange != nil && h.ThermalDynamicRange.Available {
		tdr := h.ThermalDynamicRange
		ratingColor := ratingColor(tdr.Rating)
		line("🌡️ Thermal Range", fmt.Sprintf("%.1f°C", tdr.TDRCelsius), ratingColor+"● "+tdr.Rating+colorReset)
		detail := fmt.Sprintf("%.0f°C idle → %.0f°C peak", tdr.IdleTempC, tdr.PeakTempC)
		line("   ", detail, "")
	} else {
		line("🌡️ Thermal Range", "—", "Establishing baseline")
	}

	// TRE
	if h != nil && h.ThermalRecovery != nil && h.ThermalRecovery.Available && h.ThermalRecovery.RecoveryCount > 0 {
		tre := h.ThermalRecovery
		ratingColor := ratingColor(tre.Rating)
		line("⏱️ Recovery", fmt.Sprintf("~%ds", tre.LastRecoverySec), ratingColor+"● "+tre.Rating+colorReset)
	} else {
		line("⏱️ Recovery", "—", "(no recovery events)")
	}

	// PPW
	if h != nil && h.PerfPerWatt != nil && h.PerfPerWatt.Available {
		ppw := h.PerfPerWatt
		unit := ppw.Unit
		if unit == "" {
			unit = "%/W"
		}
		line("⚡ Efficiency", fmt.Sprintf("%.1f %s", ppw.Value, unit), "")
	} else {
		line("⚡ Efficiency", "—", "(power < 1W)")
	}

	// Stability
	if h != nil && h.ThermalStability != nil && h.ThermalStability.Available && h.ThermalStability.UnderSustainedLoad {
		stab := h.ThermalStability
		ratingColor := ratingColor(stab.Rating)
		line("📊 Stability", fmt.Sprintf("±%.1f°C", stab.StabilityCelsius), ratingColor+"● "+stab.Rating+colorReset)
	} else {
		line("📊 Stability", "—", "(no sustained load)")
	}
}

// DashboardData holds either API or Prometheus-sourced data for rendering.
type DashboardData struct {
	Status     *api.StatusResponse
	Risk       *api.RiskResponse
	FromLegacy bool // true when using Prometheus fallback
}
