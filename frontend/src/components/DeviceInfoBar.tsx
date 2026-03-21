import { RiskHexBadge } from '@/components/RiskHexBadge'

export interface DeviceInfoBarProps {
  hostname: string
  hardwareLine: string
  connected: boolean
  agentVersion: string
  score: number
  severity: 'normal' | 'warning' | 'critical'
  trend: 'stable' | 'rising' | 'falling'
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
  hexAriaLabel,
}: DeviceInfoBarProps) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 min-w-0">
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0">
          <h2 className="text-sm font-semibold text-[#E8ECF4] truncate">
            {hostname}
          </h2>
          <span className="text-[11px] text-[#94A3B8] truncate">
            {hardwareLine}
          </span>
        </div>
        <div className="flex items-center gap-1.5 text-[11px] text-[#94A3B8]">
          <span
            className="w-1.5 h-1.5 rounded-full shrink-0"
            style={{
              backgroundColor: connected ? '#00C9B0' : '#EF4444',
            }}
            aria-hidden
          />
          <span>{connected ? 'Online' : 'Offline'}</span>
          <span className="text-[#64748B]" aria-hidden>
            ·
          </span>
          <span className="text-[#64748B]">keldron-agent v{agentVersion}</span>
        </div>
      </div>

      <div
        className="shrink-0 self-center sm:self-center"
        role="img"
        aria-label={hexAriaLabel}
      >
        <RiskHexBadge
          score={score}
          severity={severity}
          trend={trend}
          size="md"
        />
      </div>
    </div>
  )
}
