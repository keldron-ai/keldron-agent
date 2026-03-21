import * as React from 'react'
import { HelpCircle, Zap } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

const { useId } = React

type Rating =
  | 'normal'
  | 'excellent'
  | 'stable'
  | 'compressed'
  | 'slow'
  | 'elevated'
  | 'critical'
  | 'poor'
  | 'unstable'

interface HealthTileData {
  type: 'thermal' | 'recovery' | 'efficiency' | 'stability'
  value?: string
  idleTemp?: number
  peakTemp?: number
  rating?: Rating
  available: boolean
}

interface StatusHealth {
  thermal_dynamic_range?: {
    available: boolean
    tdr_celsius: number | null
    idle_temp_c: number | null
    peak_temp_c: number | null
    rating: string | null
  }
  thermal_recovery?: {
    available: boolean
    last_recovery_seconds: number | null
    rating: string | null
    recovery_count: number
  }
  perf_per_watt?: {
    available: boolean
    value: number | null
    unit: string
  }
  thermal_stability?: {
    available: boolean
    std_dev_celsius: number | null
    rating: string | null
    under_sustained_load: boolean
  }
}

type MetricKey = 'thermal' | 'recovery' | 'efficiency' | 'stability'

const tooltips: Record<MetricKey, string> = {
  thermal: 'Gap between idle and peak temperature. Wider is healthier.',
  recovery: 'Time to cool down after a heavy workload ends.',
  efficiency: 'GPU utilization per watt of power consumed.',
  stability: 'Temperature consistency under sustained load. Lower is better.',
}

const labels: Record<MetricKey, string> = {
  thermal: 'Thermal Range',
  recovery: 'Recovery',
  efficiency: 'Efficiency',
  stability: 'Stability',
}

const unavailableMessages: Record<MetricKey, string> = {
  thermal: 'Establishing…',
  recovery: 'No recovery data yet',
  efficiency: '(power < 1W)',
  stability: '(no sustained load)',
}

function getRatingColor(rating: Rating): string {
  switch (rating) {
    case 'normal':
    case 'excellent':
    case 'stable':
      return '#22C55E'
    case 'compressed':
    case 'slow':
    case 'elevated':
      return '#F59E0B'
    case 'critical':
    case 'poor':
    case 'unstable':
      return '#EF4444'
    default:
      return '#22C55E'
  }
}

function ThermalRangeGraphic({
  idleTemp,
  peakTemp,
  available,
}: {
  idleTemp?: number
  peakTemp?: number
  available: boolean
}) {
  const gradientId = useId()

  if (!available || idleTemp == null || peakTemp == null) {
    return (
      <div className="flex items-center justify-center h-4 opacity-30">
        <svg width="32" height="5" viewBox="0 0 40 6">
          <rect x="0" y="0" width="40" height="6" rx="3" fill="#94A3B8" />
        </svg>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-1 h-4">
      <span className="text-[8px] text-[#64748B]">{idleTemp.toFixed(1)}°</span>
      <svg width="32" height="5" viewBox="0 0 40 6">
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
      <span className="text-[8px] text-[#64748B]">{peakTemp.toFixed(1)}°</span>
    </div>
  )
}

function RecoveryCurveGraphic({ available }: { available: boolean }) {
  return (
    <div className="flex items-center justify-center h-4">
      <svg
        width="32"
        height="18"
        viewBox="0 0 40 24"
        className={available ? '' : 'opacity-30'}
      >
        <path
          d="M2 4 C10 4, 14 20, 38 20"
          fill="none"
          stroke={available ? '#00C9B0' : '#94A3B8'}
          strokeWidth="2"
          strokeLinecap="round"
        />
      </svg>
    </div>
  )
}

function StabilityWaveGraphic({
  available,
  rating,
}: {
  available: boolean
  rating?: Rating
}) {
  const isStable =
    rating === 'stable' || rating === 'normal' || rating === 'excellent'
  const amplitude = isStable ? 2 : 5
  const points = []
  for (let x = 0; x <= 40; x += 2) {
    const y = 8 + Math.sin((x / 40) * Math.PI * 4) * amplitude
    points.push(`${x},${y}`)
  }
  const pathD = `M${points.join(' L')}`

  return (
    <div className="flex items-center justify-center h-4">
      <svg
        width="32"
        height="14"
        viewBox="0 0 40 16"
        className={available ? '' : 'opacity-30'}
      >
        <path
          d={available ? pathD : 'M0,8 L40,8'}
          fill="none"
          stroke={available ? '#00C9B0' : '#94A3B8'}
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
          <HelpCircle
            size={9}
            className="text-[#64748B] hover:text-[#94A3B8] shrink-0"
            aria-hidden
          />
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

function mapHealthToMetrics(health: StatusHealth | null | undefined): HealthTileData[] {
  if (!health) {
    return [
      { type: 'thermal', available: false },
      { type: 'recovery', available: false },
      { type: 'efficiency', available: false },
      { type: 'stability', available: false },
    ]
  }
  const tdr = health.thermal_dynamic_range
  const tre = health.thermal_recovery
  const ppw = health.perf_per_watt
  const stab = health.thermal_stability

  const thermal: HealthTileData = {
    type: 'thermal',
    available: !!(tdr?.available && tdr.tdr_celsius != null),
    value: tdr?.tdr_celsius != null ? `${tdr.tdr_celsius.toFixed(0)}°C` : undefined,
    idleTemp: tdr?.idle_temp_c ?? undefined,
    peakTemp: tdr?.peak_temp_c ?? undefined,
    rating: (tdr?.rating?.toLowerCase() as Rating) ?? undefined,
  }

  const hasRecoveryData =
    tre && (tre.recovery_count > 0 || tre.last_recovery_seconds != null)
  const recovery: HealthTileData = {
    type: 'recovery',
    available: !!(hasRecoveryData && tre.last_recovery_seconds != null),
    value:
      tre?.last_recovery_seconds != null
        ? `~${tre.last_recovery_seconds}s`
        : undefined,
    rating: (tre?.rating?.toLowerCase() as Rating) ?? undefined,
  }

  const efficiency: HealthTileData = {
    type: 'efficiency',
    available: !!(ppw?.available && ppw.value != null),
    value: ppw?.value != null ? `${ppw.value.toFixed(1)} %/W` : undefined,
  }

  const stability: HealthTileData = {
    type: 'stability',
    available: !!(stab?.under_sustained_load && stab.std_dev_celsius != null),
    value:
      stab?.std_dev_celsius != null
        ? `±${stab.std_dev_celsius.toFixed(1)}°C`
        : undefined,
    rating: (stab?.rating?.toLowerCase() as Rating) ?? undefined,
  }

  return [thermal, recovery, efficiency, stability]
}

function CompactCell({ metric }: { metric: HealthTileData }) {
  return (
    <div className="flex flex-col gap-0.5 min-h-[72px]">
      <HealthTooltip content={tooltips[metric.type]} label={labels[metric.type]}>
        <span className="text-[9px] text-[#94A3B8] leading-tight">
          {labels[metric.type]}
        </span>
      </HealthTooltip>
      {metric.available ? (
        <div className="text-[12px] font-semibold text-[#E8ECF4] leading-tight truncate">
          {metric.type === 'efficiency' ? (
            <>
              {metric.value?.replace(/\s*%\/W\s*$/, '').trim()}
              <span className="text-[8px] font-normal text-[#94A3B8] ml-0.5">
                %/W
              </span>
            </>
          ) : (
            metric.value
          )}
        </div>
      ) : (
        <div className="text-[12px] font-semibold text-[#64748B] leading-tight">
          —
        </div>
      )}
      <div className="flex-1 min-h-0 flex flex-col justify-center">
        {metric.type === 'thermal' && (
          <ThermalRangeGraphic
            idleTemp={metric.idleTemp}
            peakTemp={metric.peakTemp}
            available={metric.available}
          />
        )}
        {metric.type === 'recovery' && (
          <RecoveryCurveGraphic available={metric.available} />
        )}
        {metric.type === 'efficiency' && (
          <div className="flex items-center justify-center h-4">
            <Zap
              size={14}
              className={
                metric.available ? 'text-[#00C9B0]' : 'text-[#94A3B8] opacity-30'
              }
            />
          </div>
        )}
        {metric.type === 'stability' && (
          <StabilityWaveGraphic
            available={metric.available}
            rating={metric.rating}
          />
        )}
      </div>
      <div className="flex items-center gap-1 min-h-[14px]">
        {metric.available && metric.rating && (
          <>
            <span
              className="w-1 h-1 rounded-full shrink-0"
              style={{ backgroundColor: getRatingColor(metric.rating) }}
            />
            <span
              className="text-[8px] uppercase truncate"
              style={{
                color: getRatingColor(metric.rating),
                letterSpacing: '0.03em',
              }}
            >
              {metric.rating}
            </span>
          </>
        )}
        {!metric.available && (
          <span className="text-[8px] text-[#64748B] italic leading-tight">
            {unavailableMessages[metric.type]}
          </span>
        )}
      </div>
    </div>
  )
}

export interface HealthGridProps {
  health?: StatusHealth | null
}

export function HealthGrid({ health }: HealthGridProps) {
  const metrics = mapHealthToMetrics(health)
  const hasRecoveryData = health?.thermal_recovery != null
  const displayMetrics = hasRecoveryData
    ? metrics
    : metrics.filter((m) => m.type !== 'recovery')

  return (
    <div
      className="rounded-xl border p-2"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <h4
        className="text-[10px] uppercase text-[#94A3B8] mb-2"
        style={{ letterSpacing: '0.08em' }}
      >
        Device Health
      </h4>
      <div className="grid grid-cols-2 gap-1.5">
        {displayMetrics.map((metric) => (
          <div
            key={metric.type}
            className="rounded-lg border border-white/[0.06] p-2 bg-white/[0.02]"
          >
            <CompactCell metric={metric} />
          </div>
        ))}
      </div>
    </div>
  )
}
