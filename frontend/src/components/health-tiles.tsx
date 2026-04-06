import * as React from "react"
import { HelpCircle, Zap } from "lucide-react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
const { useId } = React

type Rating =
  | "normal"
  | "excellent"
  | "stable"
  | "compressed"
  | "slow"
  | "elevated"
  | "critical"
  | "poor"
  | "unstable"
  | "good"
  | "fair"

interface HealthTileData {
  type: "thermal" | "recovery" | "efficiency" | "stability"
  value?: string
  idleTemp?: number
  peakTemp?: number
  rating?: Rating
  available: boolean
  warmingUp?: boolean
}

interface StatusHealth {
  warming_up?: boolean
  thermal_dynamic_range?: {
    available: boolean
    no_sustained_load?: boolean
    warming_up?: boolean
    avg_temp_c?: number | null
    max_temp_c?: number | null
    headroom_used_pct?: number | null
    rating: string | null
    note?: string | null
  }
  thermal_recovery?: {
    available: boolean
    no_spikes?: boolean
    spike_active?: boolean
    active_spike_seconds?: number | null
    last_recovery_seconds?: number | null
    rating: string | null
    note?: string | null
    warming_up?: boolean
  }
  perf_per_watt?: {
    available: boolean
    value: number | null
    unit: string
  }
  thermal_stability?: {
    available: boolean
    std_dev_celsius?: number | null
    rating: string | null
    warming_up?: boolean
  }
}

interface HealthTilesProps {
  metrics?: HealthTileData[]
  health?: StatusHealth | null
}

type MetricKey = "thermal" | "recovery" | "efficiency" | "stability"

const tooltips: Record<MetricKey, string> = {
  thermal: "How close to the thermal ceiling — lower is better.",
  recovery: "How quickly temperature drops below the mid-envelope threshold after a spike.",
  efficiency: "GPU utilization per watt of power consumed (30-minute window).",
  stability: "Temperature variability over the last 30 minutes. Lower is better.",
}

const labels: Record<MetricKey, string> = {
  thermal: "Thermal Range",
  recovery: "Thermal Recovery",
  efficiency: "Efficiency",
  stability: "Stability",
}

const unavailableMessages: Record<MetricKey, string> = {
  thermal: "Establishing…",
  recovery: "No data yet",
  efficiency: "(power < 1W)",
  stability: "No data yet",
}

function getRatingColor(rating: Rating): string {
  switch (rating) {
    case "normal":
    case "excellent":
    case "stable":
    case "good":
      return "#22C55E"
    case "compressed":
    case "slow":
    case "elevated":
    case "fair":
      return "#F59E0B"
    case "critical":
    case "poor":
    case "unstable":
      return "#EF4444"
    default:
      return "#22C55E"
  }
}

// Compact thermal range indicator (horizontal)
function ThermalRangeGraphic({ idleTemp, peakTemp, available }: { idleTemp?: number; peakTemp?: number; available: boolean }) {
  const gradientId = useId()

  if (!available || idleTemp == null || peakTemp == null) {
    return (
      <div className="flex items-center justify-center h-6 opacity-30">
        <svg width="40" height="6" viewBox="0 0 40 6">
          <rect x="0" y="0" width="40" height="6" rx="3" fill="#94A3B8" />
        </svg>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-1.5 h-6">
      <span className="text-[9px] text-[#64748B]">{idleTemp.toFixed(1)}°</span>
      <svg width="40" height="6" viewBox="0 0 40 6">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#475569" />
            <stop offset="25%" stopColor="#00C9B0" />
            <stop offset="75%" stopColor="#00C9B0" />
            <stop offset="100%" stopColor="#475569" />
          </linearGradient>
        </defs>
        <rect x="0" y="0" width="40" height="6" rx="3" fill={`url(#${gradientId})`} />
      </svg>
      <span className="text-[9px] text-[#64748B]">{peakTemp.toFixed(1)}°</span>
    </div>
  )
}

// Compact recovery cooldown curve
function RecoveryCurveGraphic({ available }: { available: boolean }) {
  return (
    <div className="flex items-center justify-center h-6">
      <svg width="40" height="24" viewBox="0 0 40 24" className={available ? "" : "opacity-30"}>
        <path
          d="M2 4 C10 4, 14 20, 38 20"
          fill="none"
          stroke={available ? "#00C9B0" : "#94A3B8"}
          strokeWidth="2"
          strokeLinecap="round"
        />
      </svg>
    </div>
  )
}

// Compact stability wave
function StabilityWaveGraphic({ available, rating }: { available: boolean; rating?: Rating }) {
  const isStable =
    rating === "stable" || rating === "normal" || rating === "excellent" || rating === "good"
  const amplitude = isStable ? 2 : 5
  
  const points = []
  for (let x = 0; x <= 40; x += 2) {
    const y = 8 + Math.sin((x / 40) * Math.PI * 4) * amplitude
    points.push(`${x},${y}`)
  }
  const pathD = `M${points.join(" L")}`

  return (
    <div className="flex items-center justify-center h-6">
      <svg width="40" height="16" viewBox="0 0 40 16" className={available ? "" : "opacity-30"}>
        <path
          d={available ? pathD : "M0,8 L40,8"}
          fill="none"
          stroke={available ? "#00C9B0" : "#94A3B8"}
          strokeWidth="2"
          strokeLinecap="round"
        />
      </svg>
    </div>
  )
}

function HealthTooltip({
  children,
  content,
  label,
}: {
  children: React.ReactNode
  content: string
  label: string
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-0.5 cursor-help rounded-sm text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#00C9B0]"
          aria-label={`${label}: more information`}
        >
          {children}
          <HelpCircle size={10} className="text-[#64748B] hover:text-[#94A3B8] shrink-0" aria-hidden />
        </button>
      </TooltipTrigger>
      <TooltipContent
        side="top"
        className="bg-[#0F172A] border border-white/10 text-[#E8ECF4] max-w-[240px] text-xs"
      >
        {content}
      </TooltipContent>
    </Tooltip>
  )
}

function HealthTile({ metric }: { metric: HealthTileData }) {
  return (
    <div className="flex flex-col items-center text-center px-3 py-1">
      {/* Value - smaller text */}
      {metric.available ? (
        <div className="text-[16px] font-semibold text-[#E8ECF4] leading-tight">
          {metric.type === "efficiency" ? (
            <>
              {metric.value?.replace(/\s*%\/W\s*$/, "").trim()}
              <span className="text-[10px] font-normal text-[#94A3B8] ml-0.5">%/W</span>
            </>
          ) : (
            metric.value
          )}
        </div>
      ) : (
        <div className="text-[16px] font-semibold text-[#64748B] leading-tight">—</div>
      )}

      {/* Compact graphic */}
      <div className="my-1">
        {metric.type === "thermal" && (
          <ThermalRangeGraphic
            idleTemp={metric.idleTemp}
            peakTemp={metric.peakTemp}
            available={
              metric.idleTemp != null &&
              metric.peakTemp != null &&
              metric.value !== "No sustained load"
            }
          />
        )}
        {metric.type === "recovery" && <RecoveryCurveGraphic available={metric.available} />}
        {metric.type === "efficiency" && (
          <div className="flex items-center justify-center h-6">
            <Zap size={18} className={metric.available ? "text-[#00C9B0]" : "text-[#94A3B8] opacity-30"} />
          </div>
        )}
        {metric.type === "stability" && (
          <StabilityWaveGraphic available={metric.available} rating={metric.rating} />
        )}
      </div>

      {/* Rating badge - inline with label */}
      <div className="flex items-center gap-1.5">
        {metric.available && metric.rating && (
          <>
            <span
              className="w-1.5 h-1.5 rounded-full"
              style={{ backgroundColor: getRatingColor(metric.rating) }}
            />
            <span
              className="text-[10px] uppercase"
              style={{ color: getRatingColor(metric.rating), letterSpacing: "0.03em" }}
            >
              {metric.rating}
            </span>
          </>
        )}
        {metric.available && metric.warmingUp && (
          <span className="text-[9px] text-[#64748B] italic">Warming up…</span>
        )}
        {!metric.available && (
          <span className="text-[9px] text-[#64748B] italic">{unavailableMessages[metric.type]}</span>
        )}
      </div>

      {/* Label with tooltip */}
      <HealthTooltip content={tooltips[metric.type]} label={labels[metric.type]}>
        <span className="text-[10px] text-[#94A3B8]">{labels[metric.type]}</span>
      </HealthTooltip>
    </div>
  )
}

function mapHealthToMetrics(health: StatusHealth | null | undefined): HealthTileData[] {
  if (!health) {
    return [
      { type: "thermal", available: false },
      { type: "recovery", available: false },
      { type: "efficiency", available: false },
      { type: "stability", available: false },
    ]
  }
  const wu = health.warming_up === true
  const tdr = health.thermal_dynamic_range
  const tre = health.thermal_recovery
  const ppw = health.perf_per_watt
  const stab = health.thermal_stability

  let thermal: HealthTileData
  if (tdr?.available && tdr.no_sustained_load) {
    thermal = {
      type: "thermal",
      available: true,
      value: "No sustained load",
      rating: "good",
      warmingUp: wu || tdr.warming_up === true,
    }
  } else if (tdr?.available && tdr.avg_temp_c != null && tdr.max_temp_c != null) {
    thermal = {
      type: "thermal",
      available: true,
      value: `${tdr.avg_temp_c.toFixed(0)}°C – ${tdr.max_temp_c.toFixed(0)}°C`,
      idleTemp: tdr.avg_temp_c,
      peakTemp: tdr.max_temp_c,
      rating: (tdr?.rating?.toLowerCase() as Rating) ?? undefined,
      warmingUp: wu || tdr.warming_up === true,
    }
  } else {
    thermal = { type: "thermal", available: false, warmingUp: wu }
  }

  let recovery: HealthTileData
  if (tre?.available) {
    if (tre.spike_active && tre.active_spike_seconds != null) {
      recovery = {
        type: "recovery",
        available: true,
        value: `Active — ${tre.active_spike_seconds}s`,
        rating: (tre?.rating?.toLowerCase() as Rating) ?? undefined,
        warmingUp: wu || tre.warming_up === true,
      }
    } else if (tre.no_spikes) {
      recovery = {
        type: "recovery",
        available: true,
        value: tre.note?.trim() || "No spikes detected",
        rating: (tre?.rating?.toLowerCase() as Rating) ?? "good",
        warmingUp: wu || tre.warming_up === true,
      }
    } else if (tre.last_recovery_seconds != null) {
      recovery = {
        type: "recovery",
        available: true,
        value: `~${tre.last_recovery_seconds}s`,
        rating: (tre?.rating?.toLowerCase() as Rating) ?? undefined,
        warmingUp: wu || tre.warming_up === true,
      }
    } else {
      recovery = { type: "recovery", available: false, warmingUp: wu || tre.warming_up === true }
    }
  } else {
    recovery = { type: "recovery", available: false, warmingUp: wu }
  }

  const efficiency: HealthTileData = {
    type: "efficiency",
    available: !!(ppw?.available && ppw.value != null),
    value: ppw?.value != null ? `${ppw.value.toFixed(1)} %/W` : undefined,
    warmingUp: wu,
  }

  const stability: HealthTileData = {
    type: "stability",
    available: !!(stab?.available && stab.std_dev_celsius != null),
    value:
      stab?.std_dev_celsius != null ? `±${stab.std_dev_celsius.toFixed(1)}°C` : undefined,
    rating: (stab?.rating?.toLowerCase() as Rating) ?? undefined,
    warmingUp: wu || stab?.warming_up === true,
  }

  return [thermal, recovery, efficiency, stability]
}

export function HealthTiles({ metrics: metricsProp, health }: HealthTilesProps) {
  const metrics = metricsProp ?? mapHealthToMetrics(health)

  return (
    <div className="border-t border-[rgba(148,163,184,0.1)] pt-3 mt-4">
      <h4
        className="text-[10px] uppercase text-[#94A3B8] mb-2"
        style={{ letterSpacing: "0.08em" }}
      >
        DEVICE HEALTH
      </h4>

      <div className="grid grid-cols-4 gap-4">
        {metrics.map((metric) => (
          <div key={metric.type} className="rounded-lg border border-white/[0.06] p-3 bg-white/[0.02]">
            <HealthTile metric={metric} />
          </div>
        ))}
      </div>
    </div>
  )
}
