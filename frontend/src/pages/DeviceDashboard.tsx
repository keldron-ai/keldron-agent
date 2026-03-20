import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Lock } from 'lucide-react'
import { useTelemetry } from '@/context/TelemetryContext'
import { RiskHexBadge } from '@/components/RiskHexBadge'
import { SubScoreBars } from '@/components/SubScoreBars'
import { HealthTiles } from '@/components/health-tiles'
import {
  TelemetryChart,
  type ChartEventFlash,
} from '@/components/telemetry-chart'
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion'
import { SystemInfoCard } from '@/components/SystemInfoCard'
import { ProcessTable } from '@/components/ProcessTable'

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

function formatMemoryGB(bytes: number | undefined): string {
  if (bytes == null || bytes <= 0) return '—'
  const gb = bytes / (1024 * 1024 * 1024)
  return `${gb.toFixed(0)} GB Unified Memory`
}

function severityRank(s: 'normal' | 'warning' | 'critical'): number {
  if (s === 'critical') return 2
  if (s === 'warning') return 1
  return 0
}

function getNumericDetail(
  details: Record<string, unknown> | undefined,
  key: string
): number | undefined {
  if (!details) return undefined
  const v = details[key]
  if (typeof v === 'number' && Number.isFinite(v)) return v
  return undefined
}

function getTrendText(trend: 'stable' | 'rising' | 'falling'): string {
  switch (trend) {
    case 'rising':
      return 'Trending up over last 45 minutes'
    case 'falling':
      return 'Trending down over last 45 minutes'
    default:
      return 'Stable'
  }
}

const TIME_RANGES = [
  { key: '30m' as const, label: '30m', locked: false },
  { key: '1H' as const, label: '1H', locked: true },
  { key: '6H' as const, label: '6H', locked: true },
  { key: '24H' as const, label: '24H', locked: true },
]

const TIME_RANGE_MS: Record<(typeof TIME_RANGES)[number]['key'], number> = {
  '30m': 30 * 60 * 1000,
  '1H': 60 * 60 * 1000,
  '6H': 6 * 60 * 60 * 1000,
  '24H': 24 * 60 * 60 * 1000,
}

export function DeviceDashboard() {
  const [timeRange, setTimeRange] = useState<'30m' | '1H' | '6H' | '24H'>('30m')
  const reducedMotion = usePrefersReducedMotion()
  const [tempChartFlash, setTempChartFlash] = useState<ChartEventFlash | null>(
    null
  )
  const [utilChartFlash, setUtilChartFlash] = useState<ChartEventFlash | null>(
    null
  )
  const prevComposite = useRef<'normal' | 'warning' | 'critical' | null>(null)
  const prevThrottle = useRef<boolean | undefined>(undefined)
  const prevTempSev = useRef<'normal' | 'warning' | 'critical' | null>(null)
  const {
    status,
    statusLoading,
    statusError,
    connected,
    latest,
    history,
    risk: riskDetail,
    processes,
  } = useTelemetry()

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

  const subScores = riskDetail?.sub_scores
  const throttleC = getNumericDetail(
    subScores?.thermal?.details,
    'throttle_threshold_c'
  )
  const tdpW = getNumericDetail(subScores?.power?.details, 'tdp_w')

  const windowMs = TIME_RANGE_MS[timeRange]
  const chartHistory = useMemo(() => {
    const cutoff = Date.now() - windowMs
    return {
      temperature: history.temperature.filter((p) => p.timestamp > cutoff),
      utilization: history.utilization.filter((p) => p.timestamp > cutoff),
      power: history.power.filter((p) => p.timestamp > cutoff),
      memory: history.memory.filter((p) => p.timestamp > cutoff),
      risk: history.risk.filter((p) => p.timestamp > cutoff),
    }
  }, [history, windowMs])

  const tempSeverity: 'normal' | 'warning' | 'critical' =
    throttleC != null && temp >= throttleC * 0.7
      ? temp >= throttleC
        ? 'critical'
        : 'warning'
      : 'normal'
  useEffect(() => {
    if (reducedMotion) return

    const th = telemetry?.throttle_active ?? false
    let thermalFlash: ChartEventFlash | null = null

    if (prevThrottle.current !== undefined && !prevThrottle.current && th) {
      thermalFlash = { text: '[THROTTLING]', key: Date.now() }
    }
    prevThrottle.current = th

    if (
      !thermalFlash &&
      prevTempSev.current !== null &&
      prevTempSev.current === 'normal' &&
      (tempSeverity === 'warning' || tempSeverity === 'critical')
    ) {
      thermalFlash = { text: '[HIGH TEMP]', key: Date.now() }
    }

    if (thermalFlash) {
      setTempChartFlash(thermalFlash)
    }

    prevTempSev.current = tempSeverity

    if (prevComposite.current === null) {
      prevComposite.current = hexSeverity
    } else {
      if (severityRank(hexSeverity) > severityRank(prevComposite.current)) {
        setUtilChartFlash({
          text:
            hexSeverity === 'critical' ? '[CRITICAL RISK]' : '[ELEVATED RISK]',
          key: Date.now(),
        })
      }
      prevComposite.current = hexSeverity
    }
  }, [
    reducedMotion,
    hexSeverity,
    tempSeverity,
    telemetry?.throttle_active,
  ])

  if (statusLoading && !status) {
    return (
      <div className="flex items-center justify-center min-h-[60vh] text-[#94A3B8]">
        Loading...
      </div>
    )
  }

  if (statusError) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[60vh] text-[#94A3B8] gap-4">
        <p>Failed to load: {statusError}</p>
        <p className="text-sm text-[#64748B]">
          Ensure the agent is running on port 9200
        </p>
      </div>
    )
  }

  return (
    <div className="flex-1 px-6 py-6">
      <div className="max-w-[1280px] mx-auto space-y-6">
        {/* Hero card: 3 columns + health */}
        <div
          className="rounded-xl border p-6"
          style={{
            backgroundColor: '#0F172A',
            borderColor: 'rgba(148, 163, 184, 0.1)',
          }}
        >
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
            {/* Left: Device info */}
            <div className="space-y-2">
              <h2 className="text-lg font-semibold text-[#E8ECF4]">
                {device?.hostname ?? '—'}
              </h2>
              <p className="text-sm text-[#94A3B8]">
                {device?.hardware ?? '—'} · {formatMemoryGB(status?.telemetry?.memory_total_bytes)}
              </p>
              <div className="flex items-center gap-2">
                <span
                  className="w-2 h-2 rounded-full shrink-0"
                  style={{
                    backgroundColor: connected ? '#00C9B0' : '#EF4444',
                  }}
                />
                <span className="text-sm text-[#94A3B8]">
                  {connected ? 'Online' : 'Offline'}
                </span>
              </div>
              <p className="text-xs text-[#64748B]">
                keldron-agent v{agent?.version ?? '—'}
              </p>
            </div>

            {/* Center: Hex badge + trend */}
            <div className="flex flex-col items-center justify-center">
              <RiskHexBadge
                score={score}
                severity={hexSeverity}
                trend={trend}
                size="lg"
                trendText={getTrendText(trend)}
              />
            </div>

            {/* Right: Sub-score bars */}
            <div className="flex flex-col justify-center">
              <SubScoreBars subScores={subScores} />
            </div>
          </div>

          {/* Device Health mini-cards */}
          <HealthTiles health={health} />
        </div>

        {/* Chart section: 2x2 grid + time range buttons */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex gap-1">
              {TIME_RANGES.map(({ key, label, locked }) => (
                <button
                  key={key}
                  type="button"
                  disabled={locked}
                  onClick={() => !locked && setTimeRange(key)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
                    timeRange === key
                      ? 'bg-[#00C9B0] text-[#0A0C10]'
                      : locked
                        ? 'text-[#64748B] cursor-not-allowed flex items-center gap-1'
                        : 'text-[#94A3B8] hover:bg-white/5 hover:text-[#E8ECF4]'
                  }`}
                >
                  {label}
                  {locked && <Lock size={10} />}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <TelemetryChart
              title="SoC Temperature"
              data={chartHistory.temperature}
              unit="°C"
              color="#00C9B0"
              thresholdValue={throttleC}
              thresholdLabel="Throttle"
              thresholdStrokeColor="#EF4444"
              yDomain={
                chartHistory.temperature.length > 0 && throttleC != null
                  ? [
                      Math.min(
                        Math.min(
                          ...chartHistory.temperature.map((p) => p.value)
                        ),
                        throttleC - 20
                      ),
                      Math.max(
                        Math.max(
                          ...chartHistory.temperature.map((p) => p.value)
                        ),
                        throttleC + 10,
                        100
                      ),
                    ]
                  : [0, 100]
              }
              currentValue={temp}
              currentValueSeverity={tempSeverity}
              eventFlash={tempChartFlash}
              onEventFlashEnd={() => setTempChartFlash(null)}
            />
            <TelemetryChart
              title="GPU Utilization"
              data={chartHistory.utilization}
              unit="%"
              color="#3B82F6"
              yDomain={[0, 100]}
              currentValue={util}
              eventFlash={utilChartFlash}
              onEventFlashEnd={() => setUtilChartFlash(null)}
            />
            <TelemetryChart
              title="System Power"
              data={chartHistory.power}
              unit="W"
              color="#F59E0B"
              thresholdValue={tdpW}
              thresholdLabel={tdpW != null ? `TDP ${tdpW}W` : undefined}
              thresholdStrokeColor="#F59E0B"
              yDomain={
                chartHistory.power.length > 0 && tdpW != null
                  ? [
                      0,
                      Math.max(
                        Math.max(...chartHistory.power.map((p) => p.value)),
                        tdpW * 1.1,
                        150
                      ),
                    ]
                  : [0, 150]
              }
              currentValue={power}
            />
            <TelemetryChart
              title="Unified Memory"
              data={chartHistory.memory}
              unit="%"
              color="#00E5CC"
              yDomain={[0, 100]}
              currentValue={memPct}
            />
          </div>
        </div>

        {/* System + Active Processes side by side */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <SystemInfoCard device={device ?? null} agent={agent ?? null} />
          <div>
            {processes === null ? (
              <div
                className="rounded-xl border p-5"
                style={{
                  backgroundColor: '#0F172A',
                  borderColor: 'rgba(148, 163, 184, 0.1)',
                }}
              >
                <h3 className="text-sm font-semibold text-[#E8ECF4] mb-2">
                  Active Processes
                </h3>
                <p className="text-sm text-[#64748B]">Loading...</p>
              </div>
            ) : (
              <ProcessTable
                processes={processes.processes}
                supported={processes.supported}
                note={processes.note}
              />
            )}
          </div>
        </div>

        {/* Footer: severity + Layer 1 + drill-down link */}
        <div
          className="rounded-xl border p-5 flex items-center justify-between flex-wrap gap-3"
          style={{
            backgroundColor: '#0F172A',
            borderColor: 'rgba(148, 163, 184, 0.1)',
          }}
        >
          <div className="flex items-center gap-3">
            <p className="text-sm text-[#94A3B8]">
              {getRiskSummaryText(severity, score)}
            </p>
            <span className="text-xs text-[#64748B]">·</span>
            <p className="text-xs text-[#64748B]">Layer 1 local scoring</p>
          </div>
          <Link
            to="/risk"
            className="text-sm transition-colors"
            style={{ color: '#00C9B0' }}
          >
            View Risk Details →
          </Link>
        </div>
      </div>
    </div>
  )
}
