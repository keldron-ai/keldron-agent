import { Link } from "react-router-dom"
import { HexBadge } from "./hex-badge"
import { Sparkline } from "./sparkline"

type Trend = "stable" | "rising-normal" | "rising-concerning" | "falling"
type SparklineType = "flat" | "moderate" | "rising" | "ramping" | "ceiling" | "bursty" | "idle" | "offline"
type Severity = "healthy" | "elevated" | "warning" | "critical" | "offline"
type Status = "online" | "offline"

interface Metric {
  label: string
  value: string
  trend: Trend
  sparklineType: SparklineType
  warning?: boolean
}

interface DeviceCardProps {
  id: string
  name: string
  chip: string | null
  status: Status
  score: number | null
  severity: Severity
  metrics: Metric[]
  ip: string
  lastUpdate: string
}

function TrendArrow({ trend, warning }: { trend: Trend; warning?: boolean }) {
  switch (trend) {
    case "stable":
      return <span className="text-[#64748B]">→</span>
    case "rising-normal":
      return <span className="text-[#94A3B8]">↗</span>
    case "rising-concerning":
      return <span className={warning ? "text-[#F59E0B]" : "text-[#F59E0B]"}>↑</span>
    case "falling":
      return <span className="text-[#10B981]">↓</span>
    default:
      return null
  }
}

export function DeviceCard({
  id,
  name,
  chip,
  status,
  score,
  severity,
  metrics,
  ip,
  lastUpdate,
}: DeviceCardProps) {
  const isOffline = status === "offline"
  const isWarning = severity === "warning"

  const borderClass = isWarning
    ? "border-[rgba(245,158,11,0.2)]"
    : "border-white/[0.06]"

  return (
    <Link
      to={`/device/${id}`}
      className={`
        block bg-[#0F172A] rounded-xl border ${borderClass}
        hover:border-white/[0.12] transition-colors duration-150 cursor-pointer
        ${isOffline ? "opacity-60" : ""}
      `}
    >
      {/* Row 1: Device header */}
      <div className="flex items-center justify-between px-5 pt-4">
        <div className="flex items-center gap-2">
          <span className="text-[15px] font-semibold text-[#E8ECF4]">{name}</span>
          {chip && (
            <span className="bg-white/[0.08] rounded-full px-2.5 py-0.5 text-[11px] text-[#94A3B8]">
              {chip}
            </span>
          )}
        </div>
        <span
          className={`w-2 h-2 rounded-full ${
            status === "online" 
              ? "bg-[#10B981] animate-heartbeat" 
              : "bg-[#475569]"
          }`}
        />
      </div>

      {/* Row 2: Risk score badge */}
      <div className="flex flex-col items-center py-3">
        <HexBadge score={score} severity={severity} />
        <span
          className={`mt-1.5 text-xs font-semibold ${
            severity === "healthy"
              ? "text-[#10B981]"
              : severity === "elevated"
              ? "text-[#00C9B0]"
              : severity === "warning"
              ? "text-[#F59E0B]"
              : severity === "critical"
              ? "text-[#EF4444]"
              : "text-[#475569]"
          }`}
        >
          {severity === "healthy"
            ? "Healthy"
            : severity === "elevated"
            ? "Elevated"
            : severity === "warning"
            ? "Warning"
            : severity === "critical"
            ? "Critical"
            : "Offline"}
        </span>
      </div>

      {/* Row 3: Metrics */}
      <div className="px-5 pb-4 space-y-1.5">
        {metrics.map((metric) => (
          <div key={metric.label} className="flex items-center h-7">
            <span className="text-xs text-[#64748B] w-[100px] shrink-0">
              {metric.label}
            </span>
            <div className="flex-1 px-2">
              <Sparkline
                type={metric.sparklineType}
                warning={metric.warning}
                offline={isOffline}
              />
            </div>
            <div className="flex items-center gap-1 shrink-0">
              <span
                className={`text-[13px] font-semibold ${
                  metric.warning ? "text-[#F59E0B]" : "text-[#E8ECF4]"
                }`}
              >
                {metric.value}
              </span>
              {!isOffline && metric.value !== "—" && (
                <TrendArrow trend={metric.trend} warning={metric.warning} />
              )}
            </div>
          </div>
        ))}
      </div>

      {/* Row 4: Footer */}
      <div className="px-5 py-2.5 border-t border-white/[0.04]">
        <span className="text-[11px] text-[#475569]">
          {isOffline ? `Last seen: ${lastUpdate}` : `Updated ${lastUpdate}`} · {ip}
        </span>
      </div>
    </Link>
  )
}
