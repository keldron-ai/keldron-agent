import { useEffect, useMemo, useRef, useState, type ComponentProps } from 'react'
import { Lock } from 'lucide-react'
import { useTelemetry } from '@/context/TelemetryContext'
import type { ChartEventFlash } from '@/components/telemetry-chart'
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion'
import { DeviceInfoBar } from '@/components/DeviceInfoBar'
import { ChartGrid } from '@/components/ChartGrid'
import { SubScoresPanel } from '@/components/SubScoresPanel'
import { HealthGrid } from '@/components/HealthGrid'
import { AIInsights } from '@/components/AIInsights'

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
  return `${gb.toFixed(0)} GB`
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

const TIME_RANGE_TO_QUERY: Record<'30m' | '1H' | '6H' | '24H', string> = {
  '30m': '30m',
  '1H': '1h',
  '6H': '6h',
  '24H': '24h',
}

type TimeRangeKey = keyof typeof TIME_RANGE_TO_QUERY

export function DeviceDashboard() {
  const [timeRange, setTimeRange] = useState<TimeRangeKey>('30m')
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
    refreshHistory,
  } = useTelemetry()

  useEffect(() => {
    void refreshHistory(TIME_RANGE_TO_QUERY[timeRange])
  }, [timeRange, refreshHistory])

  const telemetry = latest?.telemetry ?? status?.telemetry
  const risk = latest?.risk ?? status?.risk
  const device = status?.device
  const agent = status?.agent
  const health = status?.health ?? null
  const cloudConnected = status?.agent?.cloud_connected === true

  const timeRangeButtons = useMemo(
    () => [
      { key: '30m' as const, label: '30m', locked: false },
      { key: '1H' as const, label: '1H', locked: !cloudConnected },
      { key: '6H' as const, label: '6H', locked: !cloudConnected },
      { key: '24H' as const, label: '24H', locked: !cloudConnected },
    ],
    [cloudConnected]
  )

  useEffect(() => {
    if (!cloudConnected && timeRange !== '30m') {
      setTimeRange('30m')
    }
  }, [cloudConnected, timeRange])

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

  const chartHistory = history

  const tempSeverity: 'normal' | 'warning' | 'critical' =
    throttleC != null && temp >= throttleC * 0.7
      ? temp >= throttleC
        ? 'critical'
        : 'warning'
      : 'normal'
  useEffect(() => {
    const th = telemetry?.throttle_active ?? false

    if (reducedMotion) {
      prevThrottle.current = th
      prevTempSev.current = tempSeverity
      prevComposite.current = hexSeverity
      setTempChartFlash(null)
      setUtilChartFlash(null)
      return
    }

    let thermalFlash: ChartEventFlash | null = null

    if (prevThrottle.current !== undefined && !prevThrottle.current && th) {
      thermalFlash = { text: '[THROTTLING]', key: Date.now() }
    }
    prevThrottle.current = th

    if (
      !thermalFlash &&
      prevTempSev.current !== null &&
      severityRank(tempSeverity) > severityRank(prevTempSev.current)
    ) {
      thermalFlash = { text: '[HIGH TEMP]', key: Date.now() }
    }

    if (thermalFlash) {
      setTempChartFlash(thermalFlash)
    }

    prevTempSev.current = tempSeverity

    if (
      prevComposite.current !== null &&
      severityRank(hexSeverity) > severityRank(prevComposite.current)
    ) {
      setUtilChartFlash({
        text:
          hexSeverity === 'critical' ? '[CRITICAL RISK]' : '[ELEVATED RISK]',
        key: Date.now(),
      })
    }
    prevComposite.current = hexSeverity
  }, [
    reducedMotion,
    hexSeverity,
    tempSeverity,
    telemetry?.throttle_active,
  ])

  const hexAriaLabel = severity
    ? `Risk score ${score >= 10 ? score.toFixed(0) : score.toFixed(1)}, ${hexSeverity}`
    : 'Risk score loading'

  const modelLabel = device?.hardware ?? 'Device'

  const memoryLabel = formatMemoryGB(status?.telemetry?.memory_total_bytes)

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
    <div className="flex flex-col flex-1 min-h-0 min-h-[calc(100vh-3.5rem)] px-3 py-2 max-w-[1280px] mx-auto w-full gap-2 overflow-hidden">
      <div className="grid grid-cols-1 md:grid-cols-[minmax(0,65%)_minmax(0,35%)] md:grid-rows-[auto_auto_minmax(0,1fr)] gap-2 flex-1 min-h-0 min-w-0">
        <div className="order-1 md:order-none md:col-start-1 md:row-start-1 min-w-0">
          <DeviceInfoBar
            hostname={device?.hostname ?? '—'}
            hardware={device?.hardware ?? '—'}
            memoryLabel={memoryLabel}
            connected={connected}
            agentVersion={agent?.version ?? '—'}
            score={score}
            severity={hexSeverity}
            trend={trend}
            hexAriaLabel={hexAriaLabel}
          />
        </div>

        <div className="order-2 md:order-none md:col-start-1 md:row-start-2 flex flex-wrap items-center gap-2 min-w-0">
          <div className="flex gap-1 overflow-x-auto">
            {timeRangeButtons.map(({ key, label, locked }) => (
              <button
                key={key}
                type="button"
                disabled={locked}
                onClick={() => !locked && setTimeRange(key)}
                className={`px-2 py-0.5 text-xs font-medium rounded-md transition-colors shrink-0 ${
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
          {cloudConnected && (
            <span className="text-[10px] font-medium text-[#00C9B0] whitespace-nowrap">
              ◎ Cloud History Enabled
            </span>
          )}
        </div>

        <div className="order-3 md:order-none md:col-start-2 md:row-start-1 md:row-span-3 flex flex-col gap-2 min-h-0 min-w-0">
          <SubScoresPanel
            subScores={subScores}
            riskSummaryLine={getRiskSummaryText(severity, score)}
          />
          <HealthGrid
            health={health as ComponentProps<typeof HealthGrid>['health']}
          />
          <div className="md:self-start md:w-full min-h-0">
            <AIInsights
              temperatureC={temp}
              memoryPct={memPct}
              modelLabel={modelLabel}
            />
          </div>
        </div>

        <div className="order-6 md:order-none md:col-start-1 md:row-start-3 min-h-0 min-w-0 flex flex-col flex-1 h-full">
          <ChartGrid
            chartHistory={chartHistory}
            temp={temp}
            util={util}
            power={power}
            memPct={memPct}
            throttleC={throttleC}
            tdpW={tdpW}
            tempSeverity={tempSeverity}
            tempChartFlash={tempChartFlash}
            utilChartFlash={utilChartFlash}
            onTempFlashEnd={() => setTempChartFlash(null)}
            onUtilFlashEnd={() => setUtilChartFlash(null)}
            compactLayout
            fillChart
          />
        </div>
      </div>
    </div>
  )
}
