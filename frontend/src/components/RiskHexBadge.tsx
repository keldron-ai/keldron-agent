type Severity = 'normal' | 'warning' | 'critical'
type Trend = 'stable' | 'rising' | 'falling'
type Size = 'sm' | 'md' | 'lg'

interface RiskHexBadgeProps {
  score: number
  severity: Severity
  trend: Trend
  size?: Size
  trendText?: string
}

const SIZE_MAP = { sm: 64, md: 120, lg: 180 } as const
const SEVERITY_COLORS = {
  normal: '#22C55E',
  warning: '#F59E0B',
  critical: '#EF4444',
} as const
const SEVERITY_LABELS = {
  normal: 'NORMAL',
  warning: 'WARNING',
  critical: 'CRITICAL',
} as const

export function RiskHexBadge({
  score,
  severity,
  trend,
  size = 'md',
  trendText,
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
        ? '#22C55E'
        : '#94A3B8'

  const baseScoreFontSize = size === 'sm' ? 20 : size === 'md' ? 32 : 48
  const scoreFontSize = score >= 10 ? baseScoreFontSize * 0.8 : baseScoreFontSize
  const labelFontSize = size === 'sm' ? 8 : size === 'md' ? 10 : 12

  return (
    <div className="flex flex-col items-center">
      <svg
        width={dim}
        height={dim}
        viewBox="0 0 100 100"
        className="shrink-0"
      >
        <polygon
          points="50,5 95,27.5 95,72.5 50,95 5,72.5 5,27.5"
          fill="#0F172A"
          stroke={borderColor}
          strokeWidth="2"
        />
        <text
          x="50"
          y="48"
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
          y="68"
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
      <span
        className="mt-2 text-sm font-medium"
        style={{ color: trendColor }}
      >
        {trendArrow}
      </span>
      {trendText && (
        <span className="mt-1 text-xs text-[#94A3B8] block">
          {trendText}
        </span>
      )}
    </div>
  )
}
