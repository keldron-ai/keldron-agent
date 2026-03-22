import type { RiskSeverityBand } from '@/types/severity'

type Trend = 'stable' | 'rising' | 'falling'
type Size = 'sm' | 'md' | 'lg'

interface RiskHexBadgeProps {
  score: number
  severity: RiskSeverityBand
  trend: Trend
  size?: Size
  trendText?: string
  /** Omit trend arrow row below the hex (compact toolbar use). */
  hideTrendRow?: boolean
}

const SIZE_MAP = { sm: 64, md: 120, lg: 180 } as const
const SEVERITY_COLORS: Record<RiskSeverityBand, string> = {
  normal: '#00C9B0',
  active: '#3B82F6',
  elevated: '#F5A623',
  warning: '#FF6B35',
  critical: '#FF3B3B',
}
const SEVERITY_LABELS: Record<RiskSeverityBand, string> = {
  normal: 'NORMAL',
  active: 'ACTIVE',
  elevated: 'ELEVATED',
  warning: 'WARNING',
  critical: 'CRITICAL',
}

const BREATHE_CLASS: Record<RiskSeverityBand, string> = {
  normal: 'animate-hex-breathe-normal',
  active: 'animate-hex-breathe-active',
  elevated: 'animate-hex-breathe-elevated',
  warning: 'animate-hex-breathe-warning',
  critical: 'animate-hex-breathe-critical',
}

export function RiskHexBadge({
  score,
  severity,
  trend,
  size = 'md',
  trendText,
  hideTrendRow = false,
}: RiskHexBadgeProps) {
  const dim = SIZE_MAP[size]
  const borderColor = SEVERITY_COLORS[severity]
  const severityLabel = SEVERITY_LABELS[severity]

  const trendArrow =
    trend === 'rising' ? '▲' : trend === 'falling' ? '▼' : '—'
  const trendColor =
    trend === 'rising'
      ? borderColor
      : trend === 'falling'
        ? '#00C9B0'
        : '#94A3B8'

  /** sm: ~text-2xl (24px) at 64×64 — user units = 24 * (100 / dim) */
  const baseScoreFontSize =
    size === 'sm' ? (24 * 100) / SIZE_MAP.sm : size === 'md' ? 32 : 48
  const scoreFontSize = score >= 10 ? baseScoreFontSize * 0.8 : baseScoreFontSize
  const labelFontSize = size === 'sm' ? 8 : size === 'md' ? 10 : 12
  const scoreY = size === 'sm' ? 46 : 48
  const labelY = size === 'sm' ? 70 : 68

  const breatheClass = BREATHE_CLASS[severity]

  return (
    <div className="flex flex-col items-center">
      <div className={`shrink-0 inline-flex ${breatheClass}`}>
        <svg
          width={dim}
          height={dim}
          viewBox="0 0 100 100"
          className="shrink-0 block"
        >
        <polygon
          points="50,5 95,27.5 95,72.5 50,95 5,72.5 5,27.5"
          fill="#0F172A"
          stroke={borderColor}
          strokeWidth="2"
        />
        <text
          x="50"
          y={scoreY}
          textAnchor="middle"
          dominantBaseline="middle"
          fontWeight="bold"
          fontSize={scoreFontSize}
          fill="#E8ECF4"
          fontFamily="system-ui, sans-serif"
        >
          {score >= 10 ? score.toFixed(0) : score.toFixed(1)}
        </text>
        <text
          x="50"
          y={labelY}
          textAnchor="middle"
          dominantBaseline="middle"
          fontWeight="600"
          fontSize={labelFontSize}
          fill={borderColor}
          fontFamily="system-ui, sans-serif"
        >
          {severityLabel}
        </text>
        </svg>
      </div>
      {!hideTrendRow && (
        <span
          className="mt-2 text-sm font-medium"
          style={{ color: trendColor }}
        >
          {trendArrow}
        </span>
      )}
      {trendText && (
        <span className="mt-1 text-xs text-[#94A3B8] block">
          {trendText}
        </span>
      )}
    </div>
  )
}
