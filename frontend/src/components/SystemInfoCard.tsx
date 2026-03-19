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

interface SystemInfoCardProps {
  device: Device | null
  agent: Agent | null
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`
  const days = Math.floor(seconds / 86400)
  const rem = seconds % 86400
  const hours = Math.floor(rem / 3600)
  const mins = Math.floor((rem % 3600) / 60)
  return `${days}d ${hours}h ${mins}m`
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
}: SystemInfoCardProps) {
  if (!device && !agent) return null

  return (
    <div
      className="rounded-xl border p-5"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <h3 className="text-sm font-semibold text-[#E8ECF4] mb-4">System</h3>
      <div className="grid grid-cols-2 gap-x-6 gap-y-3">
        {device && (
          <>
            <div>
              <span className="text-xs text-[#94A3B8]">GPU Cores</span>
              <p className="text-sm text-[#E8ECF4]">—</p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">Neural Engine</span>
              <p className="text-sm text-[#E8ECF4]">—</p>
            </div>
            <div>
              <span className="text-xs text-[#94A3B8]">OS</span>
              <p className="text-sm text-[#E8ECF4]">
                {device.os} / {device.arch}
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
    </div>
  )
}
