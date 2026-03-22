type HexSeverity =
  | 'healthy'
  | 'normal'
  | 'active'
  | 'elevated'
  | 'warning'
  | 'critical'
  | 'offline'

interface LargeHexBadgeProps {
  score: number
  severity: HexSeverity
  trendText?: string
}

const BG: Record<HexSeverity, string> = {
  healthy: '#00C9B0',
  normal: '#00C9B0',
  active: '#3B82F6',
  elevated: '#F5A623',
  warning: '#FF6B35',
  critical: '#FF3B3B',
  offline: '#1E293B',
}

export function LargeHexBadge({ score, severity, trendText }: LargeHexBadgeProps) {
  const getGlowClass = () => {
    switch (severity) {
      case 'healthy':
      case 'normal':
        return 'animate-glow-healthy'
      case 'active':
        return 'animate-hex-breathe-active'
      case 'elevated':
        return 'animate-hex-breathe-elevated'
      case 'warning':
        return 'animate-glow-warning-large'
      case 'critical':
        return 'animate-glow-critical'
      default:
        return ''
    }
  }

  const getSeverityLabel = () => {
    switch (severity) {
      case 'healthy':
      case 'normal':
        return 'Normal'
      case 'active':
        return 'Active'
      case 'elevated':
        return 'Elevated'
      case 'warning':
        return 'Warning'
      case 'critical':
        return 'Critical'
      case 'offline':
        return 'Offline'
      default:
        return ''
    }
  }

  const getLabelColor = () => BG[severity]

  return (
    <div className="flex flex-col items-center">
      <div
        className={`w-24 h-[84px] flex items-center justify-center ${getGlowClass()}`}
        style={{
          backgroundColor: BG[severity],
          clipPath: 'polygon(25% 0%, 75% 0%, 100% 50%, 75% 100%, 25% 100%, 0% 50%)',
        }}
      >
        <span
          className="text-[32px] font-bold"
          style={{
            color: severity === 'critical' ? '#FFFFFF' : severity === 'offline' ? '#475569' : '#0A0C10',
          }}
        >
          {score}
        </span>
      </div>
      <span
        className="mt-2 text-sm font-semibold"
        style={{ color: getLabelColor() }}
      >
        {getSeverityLabel()}
      </span>
      {trendText && (
        <span className="mt-1 text-xs text-[#94A3B8]">
          {trendText}
        </span>
      )}
    </div>
  )
}
