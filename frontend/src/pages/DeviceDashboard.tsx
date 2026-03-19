import { Link } from 'react-router-dom'
import { useDeviceStatus } from '@/hooks/useDeviceStatus'
import { useTelemetryStream } from '@/hooks/useTelemetryStream'
import { HexBadge } from '@/components/hex-badge'
import { DataSparkline } from '@/components/DataSparkline'
import { SystemInfo } from '@/components/system-info'

function mapApiSeverityToHex(
  severity: string | undefined,
  score: number
): 'healthy' | 'elevated' | 'warning' | 'critical' | 'offline' {
  if (!severity) return 'offline'
  if (severity === 'critical' || score >= 80) return 'critical'
  if (severity === 'warning' || (score >= 60 && score < 80)) return 'warning'
  if (severity === 'normal' || score < 60) return 'healthy'
  return 'elevated'
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`
  const days = Math.floor(seconds / 86400)
  const hours = Math.round((seconds % 86400) / 3600)
  return `${days}d ${hours}h`
}

function getRiskSummaryText(
  severity: string | undefined,
  score: number
): string {
  if (!severity) return 'Loading...'
  if (severity === 'normal' && score < 60)
    return 'Normal — all metrics within thresholds'
  if (severity === 'warning' || (score >= 60 && score < 80))
    return 'Warning — thermal or power score elevated'
  if (severity === 'critical' || score >= 80)
    return 'Critical — immediate attention required'
  return 'Stable'
}

export function DeviceDashboard() {
  const { status, loading, error } = useDeviceStatus()
  const { latest, history } = useTelemetryStream()

  const telemetry = latest?.telemetry ?? status?.telemetry
  const risk = latest?.risk ?? status?.risk
  const device = status?.device
  const agent = status?.agent

  const score = risk?.composite_score ?? 0
  const severity = risk?.severity
  const trend = (risk?.trend ?? 'stable') as 'stable' | 'rising' | 'falling'
  const hexSeverity = mapApiSeverityToHex(severity, score)

  const temp = telemetry?.temperature_c ?? 0
  const util = telemetry?.gpu_utilization_pct ?? 0
  const power = telemetry?.power_draw_w ?? 0
  const memPct = telemetry?.memory_used_pct ?? 0
  const memUsed = telemetry?.memory_used_bytes ?? 0
  const memTotal = telemetry?.memory_total_bytes ?? 0
  const memStr =
    memTotal > 0
      ? `${(memUsed / 1024 / 1024 / 1024).toFixed(1)}/${(memTotal / 1024 / 1024 / 1024).toFixed(1)} GB`
      : '—'

  const systemInfo = device
    ? [
        { label: 'Hardware', value: device.hardware },
        { label: 'OS', value: device.os },
        { label: 'Adapter', value: device.adapter },
        { label: 'Behavior', value: device.behavior_class },
        {
          label: 'Uptime',
          value: formatUptime(device.uptime_seconds),
        },
        {
          label: 'Adapters',
          value: agent?.adapters_active?.join(', ') ?? '—',
        },
      ]
    : []

  if (loading && !status) {
    return (
      <div className="flex items-center justify-center min-h-[60vh] text-[#94A3B8]">
        Loading...
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[60vh] text-[#94A3B8] gap-4">
        <p>Failed to load: {error}</p>
        <p className="text-sm text-[#64748B]">
          Ensure the agent is running on port 9200
        </p>
      </div>
    )
  }

  return (
    <div className="flex-1 px-6 py-6">
      <div className="max-w-[900px] mx-auto space-y-6">
        {/* Risk hex badge */}
        <div className="flex flex-col items-center py-6">
          <HexBadge score={Math.round(score)} severity={hexSeverity} />
          <div className="flex items-center gap-2 mt-2">
            <span
              className={`text-sm font-semibold ${
                hexSeverity === 'healthy'
                  ? 'text-[#10B981]'
                  : hexSeverity === 'warning'
                    ? 'text-[#F59E0B]'
                    : hexSeverity === 'critical'
                      ? 'text-[#EF4444]'
                      : 'text-[#94A3B8]'
              }`}
            >
              {trend === 'rising' ? '▲' : trend === 'falling' ? '▼' : '—'}{' '}
              {score.toFixed(0)}
            </span>
          </div>
        </div>

        {/* 4 metric sparklines */}
        <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-5 space-y-4">
          <DataSparkline
            data={history.temperature}
            currentValue={`${temp.toFixed(0)}°C`}
            label="Temperature"
            trend={trend}
            warning={telemetry?.throttle_active}
          />
          <DataSparkline
            data={history.utilization}
            currentValue={`${util.toFixed(0)}%`}
            label="GPU Utilization"
            trend={trend}
          />
          <DataSparkline
            data={history.power}
            currentValue={`${power.toFixed(1)}W`}
            label="Power Draw"
            trend={trend}
          />
          <DataSparkline
            data={history.memory}
            currentValue={memStr}
            label="Memory"
            trend={trend}
          />
        </div>

        {/* System info + Quick risk */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <SystemInfo info={systemInfo} />
          <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-5">
            <h3 className="text-[13px] font-semibold text-[#E8ECF4] mb-4">
              Risk Summary
            </h3>
            <p className="text-[13px] text-[#94A3B8]">
              {getRiskSummaryText(severity, score)}
            </p>
            <Link
              to="/risk"
              className="mt-4 inline-flex items-center gap-1 text-sm text-[#00C9B0] hover:text-[#00E5CC] transition-colors"
            >
              View Risk Details →
            </Link>
          </div>
        </div>
      </div>
    </div>
  )
}
