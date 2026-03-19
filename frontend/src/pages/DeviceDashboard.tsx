import { Link } from 'react-router-dom'
import { useDeviceStatus } from '@/hooks/useDeviceStatus'
import { useTelemetryStream } from '@/hooks/useTelemetryStream'
import { useRiskScore } from '@/hooks/useRiskScore'
import { RiskHexBadge } from '@/components/RiskHexBadge'
import { MetricSparkline } from '@/components/MetricSparkline'
import { SystemInfoCard } from '@/components/SystemInfoCard'

function mapApiSeverity(
  severity: string | undefined,
  score: number
): 'normal' | 'warning' | 'critical' {
  if (!severity) return 'normal'
  if (severity === 'critical') return 'critical'
  if (severity === 'warning') return 'warning'
  if (score >= 80) return 'critical'
  if (score >= 60) return 'warning'
  return 'normal'
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
  const { risk: riskDetail } = useRiskScore()

  const telemetry = latest?.telemetry ?? status?.telemetry
  const risk = latest?.risk ?? status?.risk
  const device = status?.device
  const agent = status?.agent
  const health = status?.health ?? null

  const score = risk?.composite_score ?? 0
  const severity = risk?.severity
  const rawTrend = risk?.trend
  const trend: 'stable' | 'rising' | 'falling' =
    rawTrend === 'rising' || rawTrend === 'falling' ? rawTrend : 'stable'
  const hexSeverity = mapApiSeverity(severity, score)

  const temp = telemetry?.temperature_c ?? 0
  const util = telemetry?.gpu_utilization_pct ?? 0
  const power = telemetry?.power_draw_w ?? 0
  const memPct = telemetry?.memory_used_pct ?? 0

  const throttleC = riskDetail?.sub_scores?.thermal?.details
    ?.throttle_threshold_c as number | undefined
  const tdpW = riskDetail?.sub_scores?.power?.details?.tdp_w as
    | number
    | undefined

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
      <div className="max-w-[1280px] mx-auto space-y-6">
        {/* Risk hex badge */}
        <div className="flex flex-col items-center py-6">
          <RiskHexBadge
            score={score}
            severity={hexSeverity}
            trend={trend}
            size="lg"
          />
        </div>

        {/* 4 metric sparklines */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <MetricSparkline
            label="Temperature"
            value={temp}
            unit="°C"
            history={history.temperature}
            thresholdValue={throttleC}
            thresholdLabel="Throttle"
          />
          <MetricSparkline
            label="GPU Utilization"
            value={util}
            unit="%"
            history={history.utilization}
            min={0}
            max={100}
          />
          <MetricSparkline
            label="Power Draw"
            value={power}
            unit="W"
            history={history.power}
            thresholdValue={tdpW}
            thresholdLabel="TDP"
          />
          <MetricSparkline
            label="Memory"
            value={memPct}
            unit="%"
            history={history.memory}
            min={0}
            max={100}
          />
        </div>

        {/* System info card */}
        <SystemInfoCard device={device ?? null} agent={agent ?? null} health={health} />

        {/* Severity summary + drill-down link */}
        <div
          className="rounded-xl border p-5 flex items-center justify-between"
          style={{
            backgroundColor: '#0F172A',
            borderColor: 'rgba(148, 163, 184, 0.1)',
          }}
        >
          <p className="text-sm text-[#94A3B8]">
            {getRiskSummaryText(severity, score)}
          </p>
          <Link
            to="/risk"
            className="text-sm text-[#00C9B0] hover:text-[#00E5CC] transition-colors"
          >
            View Risk Details →
          </Link>
        </div>
      </div>
    </div>
  )
}
