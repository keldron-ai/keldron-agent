import { RiskHexBadge } from '@/components/RiskHexBadge'

export interface DeviceInfoBarProps {
  hostname: string
  hardware: string
  memoryLabel: string
  connected: boolean
  agentVersion: string
  score: number
  severity: 'normal' | 'warning' | 'critical'
  trend: 'stable' | 'rising' | 'falling'
  hexAriaLabel: string
}

export function DeviceInfoBar({
  hostname,
  hardware,
  memoryLabel,
  connected,
  agentVersion,
  score,
  severity,
  trend,
  hexAriaLabel,
}: DeviceInfoBarProps) {
  return (
    <div className="flex flex-row items-center justify-between gap-2 min-w-0 max-h-[64px]">
      <p className="min-w-0 flex-1 text-[11px] sm:text-xs text-[#94A3B8] truncate leading-tight">
        <span className="font-semibold text-[#E8ECF4]">{hostname}</span>
        <span className="text-[#64748B]"> · </span>
        <span>{hardware}</span>
        <span className="text-[#64748B]"> · </span>
        <span>{memoryLabel}</span>
        <span className="text-[#64748B]"> · </span>
        <span
          className="inline-block w-1.5 h-1.5 rounded-full align-middle mr-1"
          style={{
            backgroundColor: connected ? '#00C9B0' : '#EF4444',
          }}
          aria-hidden
        />
        <span>{connected ? 'Online' : 'Offline'}</span>
        <span className="text-[#64748B]"> · </span>
        <span className="text-[#64748B]">keldron-agent v{agentVersion}</span>
      </p>

      <div
        className="shrink-0 overflow-hidden rounded-md leading-none"
        role="img"
        aria-label={hexAriaLabel}
      >
        <RiskHexBadge
          score={score}
          severity={severity}
          trend={trend}
          size="sm"
          hideTrendRow
        />
      </div>
    </div>
  )
}
