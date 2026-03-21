import { Link } from 'react-router-dom'
import { RiskHexBadge } from '@/components/RiskHexBadge'

export interface DeviceInfoBarProps {
  hostname: string
  hardwareLine: string
  connected: boolean
  agentVersion: string
  score: number
  severity: 'normal' | 'warning' | 'critical'
  trend: 'stable' | 'rising' | 'falling'
  trendText: string
  severityLabel: string
  trendShortLabel: string
  hexAriaLabel: string
}

export function DeviceInfoBar({
  hostname,
  hardwareLine,
  connected,
  agentVersion,
  score,
  severity,
  trend,
  trendText,
  severityLabel,
  trendShortLabel,
  hexAriaLabel,
}: DeviceInfoBarProps) {
  const sevColor =
    severity === 'normal'
      ? '#00C9B0'
      : severity === 'warning'
        ? '#F59E0B'
        : '#EF4444'

  return (
    <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3 min-w-0">
      <div className="space-y-1 min-w-0 flex-1">
        <h2 className="text-base font-semibold text-[#E8ECF4] truncate">
          {hostname}
        </h2>
        <p className="text-xs text-[#94A3B8] truncate">{hardwareLine}</p>
        <div className="flex items-center gap-2">
          <span
            className="w-2 h-2 rounded-full shrink-0"
            style={{
              backgroundColor: connected ? '#00C9B0' : '#EF4444',
            }}
          />
          <span className="text-xs text-[#94A3B8]">
            {connected ? 'Online' : 'Offline'}
          </span>
        </div>
        <p className="text-[11px] text-[#64748B]">
          keldron-agent v{agentVersion}
        </p>
      </div>

      <div className="flex flex-col items-center sm:items-end gap-1 shrink-0 self-center sm:self-start">
        <Link
          to="/risk"
          title="View risk details"
          aria-label={hexAriaLabel}
          className="rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-[#00C9B0]/50 focus-visible:ring-offset-2 focus-visible:ring-offset-[#0F172A] cursor-pointer transition-transform duration-200 motion-safe:hover:scale-[1.03]"
        >
          <RiskHexBadge
            score={score}
            severity={severity}
            trend={trend}
            size="lg"
            trendText={trendText}
          />
        </Link>
        <div className="text-center sm:text-right space-y-0.5">
          <p className="text-[10px] font-bold uppercase tracking-wider" style={{ color: sevColor }}>
            {severityLabel}
          </p>
          <p className="text-[11px] text-[#94A3B8]">{trendShortLabel}</p>
        </div>
      </div>
    </div>
  )
}
