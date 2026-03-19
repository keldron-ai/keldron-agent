import { HelpCircle, Lock } from "lucide-react"
import { useState } from "react"

type Rating = "normal" | "excellent" | "stable" | "compressed" | "slow" | "elevated" | "critical" | "poor" | "unstable"

interface HealthMetric {
  type: "thermal" | "recovery" | "efficiency" | "stability"
  value?: string
  detail?: string
  rating?: Rating
  available: boolean
  underSustainedLoad?: boolean
}

interface DeviceHealthProps {
  metrics: HealthMetric[]
  variant?: "card" | "section"
}

const tooltips: Record<string, string> = {
  thermal: "Gap between idle and peak temperature. Wider is healthier.",
  recovery: "Time to cool down after a heavy workload ends.",
  efficiency: "GPU utilization per watt of power consumed.",
  stability: "Temperature consistency under sustained load. Lower is better.",
}

const labels: Record<string, string> = {
  thermal: "Thermal Range",
  recovery: "Recovery",
  efficiency: "Efficiency",
  stability: "Stability",
}

function getRatingColor(rating: Rating): string {
  switch (rating) {
    case "normal":
    case "excellent":
    case "stable":
      return "#22C55E"
    case "compressed":
    case "slow":
    case "elevated":
      return "#F59E0B"
    case "critical":
    case "poor":
    case "unstable":
      return "#EF4444"
    default:
      return "#22C55E"
  }
}

function HealthMetricRow({ metric }: { metric: HealthMetric }) {
  const [showTooltip, setShowTooltip] = useState(false)

  // Recovery, Efficiency, and Stability are hidden when not available
  if (!metric.available && metric.type !== "thermal") {
    return null
  }

  // Stability only shows when under sustained load
  if (metric.type === "stability" && !metric.underSustainedLoad) {
    return null
  }

  const showTrendLink = metric.type === "thermal" || metric.type === "recovery"

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        {/* Label with help icon */}
        <div className="flex items-center gap-1.5 relative">
          <span className="text-xs text-[#94A3B8]">{labels[metric.type]}</span>
          <button
            className="text-[#64748B] hover:text-[#94A3B8] transition-colors"
            aria-label={`${labels[metric.type]} info`}
            onMouseEnter={() => setShowTooltip(true)}
            onMouseLeave={() => setShowTooltip(false)}
            onFocus={() => setShowTooltip(true)}
            onBlur={() => setShowTooltip(false)}
          >
            <HelpCircle size={14} />
          </button>
          {showTooltip && (
            <div className="absolute left-0 top-6 z-10 w-48 px-2.5 py-2 bg-[#1E293B] border border-white/10 rounded-md text-[11px] text-[#E8ECF4] leading-relaxed shadow-lg">
              {tooltips[metric.type]}
            </div>
          )}
        </div>

        {/* Value */}
        <div className="text-right">
          {metric.available ? (
            <span className="text-[13px] text-[#E8ECF4]">{metric.value ?? "—"}</span>
          ) : (
            <span className="text-[13px] text-[#64748B]">—</span>
          )}
        </div>
      </div>

      {/* Detail line */}
      {metric.type === "thermal" && (
        <div className="text-right">
          {metric.available ? (
            <span className="text-[12px] text-[#64748B]">{metric.detail ?? "Establishing baseline..."}</span>
          ) : (
            <span className="text-[12px] text-[#64748B] italic">Establishing baseline...</span>
          )}
        </div>
      )}

      {/* Rating badge and trend link */}
      {metric.available && (
        <div className="flex items-center justify-between">
          <div />
          <div className="flex items-center gap-3">
            {metric.rating && (
              <div className="flex items-center gap-1.5">
                <span
                  className="w-1.5 h-1.5 rounded-full"
                  style={{ backgroundColor: getRatingColor(metric.rating) }}
                />
                <span
                  className="text-[11px] uppercase tracking-wide"
                  style={{ color: getRatingColor(metric.rating), letterSpacing: "0.05em" }}
                >
                  {metric.rating}
                </span>
              </div>
            )}
            {showTrendLink && (
              <span className="flex items-center gap-1 text-[11px] text-[#64748B]" aria-label="Trend (locked)">
                <span>Trend</span>
                <span>→</span>
                <Lock size={10} />
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export function DeviceHealth({ metrics, variant = "card" }: DeviceHealthProps) {
  const visibleMetrics = metrics.filter((m) => {
    if (!m.available && m.type !== "thermal") return false
    if (m.type === "stability" && !m.underSustainedLoad) return false
    return true
  })

  if (variant === "section") {
    return (
      <div className="pt-4 mt-4 border-t border-[rgba(148,163,184,0.15)]">
        <h4 className="text-[11px] uppercase tracking-wide text-[#94A3B8] mb-4" style={{ letterSpacing: "0.05em" }}>
          Device Health
        </h4>
        <div className="space-y-4">
          {visibleMetrics.map((metric) => (
            <HealthMetricRow key={metric.type} metric={metric} />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-5 hover:border-white/[0.12] transition-colors h-full">
      <h3 className="text-[13px] font-semibold text-[#E8ECF4] mb-4">Device Health</h3>
      <div className="space-y-4">
        {visibleMetrics.map((metric) => (
          <HealthMetricRow key={metric.type} metric={metric} />
        ))}
      </div>
    </div>
  )
}
