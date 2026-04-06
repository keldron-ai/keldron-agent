// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"

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

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visibleLen returns the terminal display width of s, ignoring ANSI escape
// codes. Uses go-runewidth for correct emoji and CJK character handling.
func visibleLen(s string) int {
	return runewidth.StringWidth(ansiRe.ReplaceAllString(s, ""))
}

// padRight pads s with spaces to visible width n, truncating with "…" if too long.
// ANSI escape sequences are excluded from width calculation.
func padRight(s string, n int) string {
	vl := visibleLen(s)
	if vl > n {
		// Truncate by visible characters — strip ANSI, truncate, lose color.
		plain := ansiRe.ReplaceAllString(s, "")
		runes := []rune(plain)
		return string(runes[:n-1]) + "…"
	}
	return s + strings.Repeat(" ", n-vl)
}

// padRightNoTruncate pads s with spaces to visible width n. Never truncates.
// Use for dashboard box lines so full text (e.g. "Establishing baseline") renders.
func padRightNoTruncate(s string, n int) string {
	vl := visibleLen(s)
	if vl >= n {
		return s
	}
	return s + strings.Repeat(" ", n-vl)
}

const boxInnerWidth = 50 // chars between ║ and ║; matches top border (╔ + 50×═ + ╗)

// boxLine returns a full box content line: ║ + content (padded to boxInnerWidth) + ║.
// Content may include ANSI codes; padding uses visible length.
func boxLine(content string) string {
	return "║" + padRightNoTruncate(content, boxInnerWidth) + "║"
}

// ratingColor returns ANSI color for health/risk rating.
func ratingColor(rating string) string {
	switch strings.ToLower(rating) {
	case "healthy", "normal", "excellent", "stable", "good":
		return colorGreen
	case "active":
		return colorCyan
	case "compressed", "slow", "elevated", "warning", "fair":
		return colorYellow
	case "critical", "poor", "unstable":
		return colorRed
	default:
		return colorDim
	}
}

// riskColor returns ANSI color for risk score using API band cut points (active, elevated, warning, critical).
func riskColor(score float64, active, elevated, warning, critical float64) string {
	if score >= critical {
		return colorRed
	}
	if score >= warning {
		return colorYellow
	}
	if score >= elevated {
		return colorYellow
	}
	if score >= active {
		return colorCyan
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
	var active, elevated, warning, critical float64 = 30, 50, 70, 90
	subScores := &api.SubScores{}
	if risk != nil {
		active = risk.Thresholds.Active
		elevated = risk.Thresholds.Elevated
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

	boxTop := "╔" + strings.Repeat("═", boxInnerWidth) + "╗"
	boxMid := "╠" + strings.Repeat("═", boxInnerWidth) + "╣"
	boxBot := "╚" + strings.Repeat("═", boxInnerWidth) + "╝"

	if !opts.Quiet {
		fmt.Fprintln(w, boxTop)
		hostname := d.Hostname
		if hostname == "" {
			hostname = "Unknown"
		}
		fmt.Fprintln(w, boxLine("  🖥️  "+hostname))
		hw := d.Hardware
		if hw == "" {
			hw = "unknown"
		}
		adapter := d.Adapter
		if adapter == "" {
			adapter = "unknown"
		}
		header2 := fmt.Sprintf("%s · %s · %s · v%s", hw, d.OS, adapter, a.Version)
		fmt.Fprintln(w, boxLine("  "+header2))
		fmt.Fprintln(w, boxMid)

		// TELEMETRY
		fmt.Fprintln(w, boxLine("  TELEMETRY"))
		tempStr := "—"
		if t.TemperatureC > 0 {
			tempStr = fmt.Sprintf("%.1f°C", t.TemperatureC)
		}
		thermalState := t.ThermalState
		if thermalState == "" {
			thermalState = "nominal"
		}
		telLine := fmt.Sprintf("  🌡️ Temperature     %-8s  %s", tempStr, thermalState)
		fmt.Fprintln(w, boxLine(telLine))
		utilBar := utilizationBar(t.GPUUtilizationPct)
		utilLine := fmt.Sprintf("  ⚡ GPU Utilization  %s %5.1f%%", utilBar, t.GPUUtilizationPct)
		fmt.Fprintln(w, boxLine(utilLine))
		powerStr := "—"
		if t.PowerDrawW > 0 {
			powerStr = fmt.Sprintf("%.2fW", t.PowerDrawW)
		}
		powerLine := fmt.Sprintf("  🔌 Power Draw      %s", powerStr)
		fmt.Fprintln(w, boxLine(powerLine))
		memStr := "—"
		if t.MemoryTotalBytes > 0 {
			usedGB := float64(t.MemoryUsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(t.MemoryTotalBytes) / (1024 * 1024 * 1024)
			memStr = fmt.Sprintf("%.1f / %.1f GB   %5.1f%%", usedGB, totalGB, t.MemoryUsedPct)
		}
		memLine := fmt.Sprintf("  🧠 Memory          %s", memStr)
		fmt.Fprintln(w, boxLine(memLine))
		fmt.Fprintln(w, boxMid)

		// RISK ANALYSIS
		riskHeader := "  RISK ANALYSIS" + strings.Repeat(" ", 26) + "Score  Wt"
		fmt.Fprintln(w, boxLine(riskHeader))
		sevColor := riskColor(r.CompositeScore, active, elevated, warning, critical)
		sevDot := ratingColor(r.Severity)
		compStr := fmt.Sprintf("%.2f", r.CompositeScore)
		if r.CompositeScore >= 10 {
			compStr = fmt.Sprintf("%.1f", r.CompositeScore)
		}
		compLine := fmt.Sprintf("  🎯 Composite        %s%-6s%s  %s● %s%s", sevColor, compStr, colorReset, sevDot, r.Severity, colorReset)
		fmt.Fprintln(w, boxLine(compLine))
		thermLine := fmt.Sprintf("     Thermal          %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Thermal.Score), subScores.Thermal.Weight)
		fmt.Fprintln(w, boxLine(thermLine))
		powerRiskLine := fmt.Sprintf("     Power            %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Power.Score), subScores.Power.Weight)
		fmt.Fprintln(w, boxLine(powerRiskLine))
		volLine := fmt.Sprintf("     Volatility       %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Volatility.Score), subScores.Volatility.Weight)
		fmt.Fprintln(w, boxLine(volLine))
		riskMemLine := fmt.Sprintf("     Memory           %-6s  (×%.2f)", fmt.Sprintf("%.2f", subScores.Memory.Score), subScores.Memory.Weight)
		fmt.Fprintln(w, boxLine(riskMemLine))
		trendStr := fmt.Sprintf("  Trend: %s (Δ %+.2f)", trendSymbol(r.Trend), r.TrendDelta)
		fmt.Fprintln(w, boxLine(trendStr))
		fmt.Fprintln(w, boxMid)

		// DEVICE HEALTH
		fmt.Fprintln(w, boxLine("  DEVICE HEALTH"))
		renderHealthSection(w, status.Health, t.PowerDrawW)
		fmt.Fprintln(w, boxMid)

		// Footer
		uptimeStr := formatUptime(d.UptimeSeconds)
		pollStr := fmt.Sprintf("%ds", a.PollIntervalS)
		cloudStr := "✗"
		if a.CloudConnected {
			cloudStr = "✓"
		}
		footer := fmt.Sprintf("  Uptime: %s · Poll: %s · Cloud: %s", uptimeStr, pollStr, cloudStr)
		fmt.Fprintln(w, boxLine(footer))
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

func renderHealthSection(w io.Writer, h *health.DeviceHealthSnapshot, powerW float64) {
	line := func(icon, val, status string) {
		const labelCol = 20 // fixed display-width column for icon+label
		iconW := visibleLen(icon)
		pad := labelCol - iconW
		if pad < 1 {
			pad = 1
		}
		content := "  " + icon + strings.Repeat(" ", pad) + val
		if status != "" {
			content += "  " + status
		}
		fmt.Fprintln(w, boxLine(content))
	}

	// Thermal Range (headroom compression)
	if h != nil && h.ThermalDynamicRange != nil && h.ThermalDynamicRange.Available {
		tdr := h.ThermalDynamicRange
		if tdr.NoSustainedLoad {
			line("🌡️ Thermal Range", "—", "No sustained load")
		} else {
			ratingCol := ratingColor(tdr.Rating)
			line("🌡️ Thermal Range", fmt.Sprintf("%.0f°C – %.0f°C", tdr.AvgTempC, tdr.MaxTempC), ratingCol+"● "+tdr.Rating+colorReset)
			detail := fmt.Sprintf("headroom %.0f%% of envelope", tdr.HeadroomUsedPct)
			line("   ", detail, "")
			if h.WarmingUp {
				line("   ", "", "(warming up)")
			}
		}
	} else {
		line("🌡️ Thermal Range", "—", "Establishing baseline")
	}

	// Thermal Recovery
	if h != nil && h.ThermalRecovery != nil && h.ThermalRecovery.Available {
		tre := h.ThermalRecovery
		ratingCol := ratingColor(tre.Rating)
		if tre.SpikeActive {
			line("⏱️ Thermal Recovery", fmt.Sprintf("Active — %ds", tre.ActiveSpikeSec), ratingCol+"● "+tre.Rating+colorReset)
		} else if tre.NoSpikes {
			line("⏱️ Thermal Recovery", "—", tre.Note)
		} else {
			line("⏱️ Thermal Recovery", fmt.Sprintf("~%ds", tre.LastRecoverySec), ratingCol+"● "+tre.Rating+colorReset)
		}
	} else {
		line("⏱️ Thermal Recovery", "—", "(no data)")
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
	if h != nil && h.ThermalStability != nil && h.ThermalStability.Available {
		stab := h.ThermalStability
		ratingCol := ratingColor(stab.Rating)
		line("📊 Stability", fmt.Sprintf("±%.1f°C", stab.StdDevCelsius), ratingCol+"● "+stab.Rating+colorReset)
		if h.WarmingUp {
			line("   ", "", "(warming up)")
		}
	} else {
		line("📊 Stability", "—", "(no data)")
	}
}

// DashboardData holds either API or Prometheus-sourced data for rendering.
type DashboardData struct {
	Status     *api.StatusResponse
	Risk       *api.RiskResponse
	FromLegacy bool // true when using Prometheus fallback
}
