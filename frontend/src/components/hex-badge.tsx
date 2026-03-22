type HexSeverity =
  | 'healthy'
  | 'normal'
  | 'active'
  | 'elevated'
  | 'warning'
  | 'critical'
  | 'offline'

interface HexBadgeProps {
  score: number | null
  severity: HexSeverity
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

export function HexBadge({ score, severity }: HexBadgeProps) {
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
        return 'animate-glow-warning'
      case 'critical':
        return 'animate-glow-critical'
      default:
        return ''
    }
  }

  return (
    <div
      className={`w-16 h-14 flex items-center justify-center ${getGlowClass()}`}
      style={{
        backgroundColor: BG[severity],
        clipPath: 'polygon(25% 0%, 75% 0%, 100% 50%, 75% 100%, 25% 100%, 0% 50%)',
      }}
    >
      <span
        className="text-[22px] font-bold"
        style={{
          color: severity === 'critical' ? '#FFFFFF' : severity === 'offline' ? '#475569' : '#0A0C10',
        }}
      >
        {score !== null ? score : '—'}
      </span>
    </div>
  )
}
