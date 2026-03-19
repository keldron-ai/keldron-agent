import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

interface Device {
  hostname: string
  adapter: string
  hardware: string
  behavior_class: string
  os: string
  arch: string
  uptime_seconds: number
}

interface Agent {
  version: string
  poll_interval_s: number
  adapters_active: string[]
  cloud_connected: boolean
}

interface Health {
  thermal_dynamic_range?: {
    available: boolean
    tdr_celsius: number | null
    idle_temp_c: number | null
    peak_temp_c: number | null
    rating: string | null
    idle_sample_count: number
    peak_sample_count: number
    window_hours: number
  }
  thermal_recovery?: {
    available: boolean
    last_recovery_seconds: number | null
    last_peak_temp_c: number | null
    last_baseline_temp_c: number | null
    rating: string | null
    recovery_count: number
    session_avg_seconds: number | null
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
    sample_count: number
    window_minutes: number
  }
}

interface SystemInfoCardProps {
  device: Device | null
  agent: Agent | null
  health: Health | null
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`
  const days = Math.floor(seconds / 86400)
  const hours = Math.round((seconds % 86400) / 3600)
  const mins = Math.round(((seconds % 86400) % 3600) / 60)
  return `${days}d ${hours}h ${mins}m`
}

function ratingColor(rating: string | null): string {
  if (!rating) return '#94A3B8'
  const r = rating.toLowerCase()
  if (r === 'healthy' || r === 'normal' || r === 'excellent' || r === 'stable')
    return '#22C55E'
  if (r === 'compressed' || r === 'elevated') return '#F59E0B'
  if (r === 'critical') return '#EF4444'
  return '#94A3B8'
}

function InfoTooltip({ children, content }: { children: React.ReactNode; content: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {children}
      </TooltipTrigger>
      <TooltipContent
        side="top"
        className="bg-[#0F172A] border border-white/10 text-[#E8ECF4] max-w-[240px]"
      >
        {content}
      </TooltipContent>
    </Tooltip>
  )
}

export function SystemInfoCard({
  device,
  agent,
  health,
}: SystemInfoCardProps) {
  if (!device && !agent) return null

  const tdr = health?.thermal_dynamic_range
  const tre = health?.thermal_recovery
  const ppw = health?.perf_per_watt
  const stab = health?.thermal_stability

  return (
    <div
      className="rounded-xl border p-5"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <div className="grid grid-cols-2 gap-x-6 gap-y-3">
        {device && (
          <>
            <div>
              <span className="text-xs text-[#94A3B8]">Hardware</span>
              <p className="text-sm font-medium text-[#E8ECF4]">
                {device.hardware || '—'}
              </p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">Adapter</span>
              <p className="text-sm">
                <span className="inline-flex items-center rounded-full bg-white/10 px-2 py-0.5 text-xs font-medium text-[#E8ECF4]">
                  {device.adapter || '—'}
                </span>
              </p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">OS / Arch</span>
              <p className="text-sm text-[#E8ECF4]">
                {device.os} / {device.arch}
              </p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">Behavior Class</span>
              <p className="text-sm text-[#E8ECF4] flex items-center gap-1">
                {device.behavior_class || '—'}
                <InfoTooltip content="Describes how this device manages thermal and power (e.g., SOC-integrated vs discrete GPU).">
                  <span className="cursor-help text-[#64748B] hover:text-[#94A3B8]">
                    ?
                  </span>
                </InfoTooltip>
              </p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">Uptime</span>
              <p className="text-sm text-[#E8ECF4]">
                {formatUptime(device.uptime_seconds)}
              </p>
            </div>
          </>
        )}
        {agent && (
          <>
            <div>
              <span className="text-xs text-[#94A3B8]">Agent Version</span>
              <p className="text-sm text-[#E8ECF4]">v{agent.version}</p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">Poll Interval</span>
              <p className="text-sm text-[#E8ECF4]">{agent.poll_interval_s}s</p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">Cloud</span>
              <p
                className="text-sm"
                style={{
                  color: agent.cloud_connected ? '#00C9B0' : '#94A3B8',
                }}
              >
                {agent.cloud_connected ? 'Connected' : 'Not connected'}
              </p>
            </div>
          </>
        )}
      </div>

      {health && (tdr || tre || ppw || stab) && (
        <>
          <div
            className="my-4 border-t"
            style={{ borderColor: 'rgba(148, 163, 184, 0.1)' }}
          />
          <h4 className="text-xs font-semibold text-[#94A3B8] mb-3">
            Device Health
          </h4>
          <div className="space-y-2">
            {tdr && (
              <div className="flex items-center gap-2">
                {tdr.available ? (
                  <span
                    className="text-sm"
                    style={{ color: ratingColor(tdr.rating ?? null) }}
                  >
                    Thermal Range:{' '}
                    {tdr.tdr_celsius != null
                      ? `${tdr.tdr_celsius.toFixed(0)}°C`
                      : '—'}{' '}
                    {tdr.idle_temp_c != null && tdr.peak_temp_c != null
                      ? `(${tdr.idle_temp_c.toFixed(0)}°C idle → ${tdr.peak_temp_c.toFixed(0)}°C peak)`
                      : ''}
                  </span>
                ) : (
                  <span className="text-sm text-[#94A3B8]">
                    Thermal Range: Establishing baseline...
                  </span>
                )}
                <InfoTooltip content="Thermal Range measures the gap between your idle and peak temperatures. A wider range means healthier cooling.">
                  <span className="cursor-help text-[#64748B] hover:text-[#94A3B8]">
                    ?
                  </span>
                </InfoTooltip>
              </div>
            )}

            {tre?.available && tre.last_recovery_seconds != null && tre.recovery_count > 0 && (
              <div className="flex items-center gap-2">
                <span
                  className="text-sm"
                  style={{ color: ratingColor(tre.rating ?? null) }}
                >
                  Recovery Time: ~{tre.last_recovery_seconds}s
                  {tre.rating ? ` (${tre.rating})` : ''}
                </span>
                <InfoTooltip content="Time for the device to cool down after a thermal spike. Shorter is better.">
                  <span className="cursor-help text-[#64748B] hover:text-[#94A3B8]">
                    ?
                  </span>
                </InfoTooltip>
              </div>
            )}

            {ppw?.available && ppw.value != null && (
              <div className="flex items-center gap-2">
                <span className="text-sm text-[#94A3B8]">
                  Efficiency: {ppw.value.toFixed(1)} {ppw.unit}
                </span>
                <InfoTooltip content="Performance per watt — higher means more efficient workload execution.">
                  <span className="cursor-help text-[#64748B] hover:text-[#94A3B8]">
                    ?
                  </span>
                </InfoTooltip>
              </div>
            )}

            {stab?.available && stab.under_sustained_load && stab.std_dev_celsius != null && (
              <div className="flex items-center gap-2">
                <span
                  className="text-sm"
                  style={{ color: ratingColor(stab.rating ?? null) }}
                >
                  Stability: ±{stab.std_dev_celsius.toFixed(1)}°C
                  {stab.rating ? ` (${stab.rating})` : ''}
                </span>
                <InfoTooltip content="Temperature variance during sustained load. Lower variance indicates more stable thermal behavior.">
                  <span className="cursor-help text-[#64748B] hover:text-[#94A3B8]">
                    ?
                  </span>
                </InfoTooltip>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}
