import { Link } from 'react-router-dom'
import { useRiskScore } from '@/hooks/useRiskScore'
import { useTelemetryStream } from '@/hooks/useTelemetryStream'
import { useProcesses } from '@/hooks/useProcesses'
import { LargeHexBadge } from '@/components/large-hex-badge'
import { RiskBreakdown } from '@/components/risk-breakdown'
import { TelemetryChart } from '@/components/telemetry-chart'
import { SystemInfo } from '@/components/system-info'
import { ActiveProcesses } from '@/components/active-processes'

function mapApiSeverityToHex(
  severity: string | undefined,
  score: number
): 'healthy' | 'elevated' | 'warning' | 'critical' | 'offline' {
  if (!severity) return 'offline'
  if (severity === 'critical') return 'critical'
  if (severity === 'elevated') return 'elevated'
  if (severity === 'warning') return 'warning'
  if (severity === 'normal') return 'healthy'
  if (score >= 80) return 'critical'
  if (score >= 60) return 'warning'
  return 'healthy'
}

function formatRuntime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`
  const hours = Math.floor(seconds / 3600)
  const mins = Math.round((seconds % 3600) / 60)
  return `${hours}h ${mins}m`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export function RiskDrilldown() {
  const { risk } = useRiskScore()
  const { history } = useTelemetryStream()
  const processes = useProcesses()

  const score = risk?.composite?.score ?? 0
  const severity = risk?.composite?.severity
  const hexSeverity = mapApiSeverityToHex(severity, score)

  const subScores = risk?.sub_scores
    ? [
        {
          label: 'Thermal margin',
          value: Math.round(risk.sub_scores.thermal.score),
          maxValue: 100,
        },
        {
          label: 'Power headroom',
          value: Math.round(risk.sub_scores.power.score),
          maxValue: 100,
        },
        {
          label: 'Load volatility',
          value: Math.round(risk.sub_scores.volatility.score),
          maxValue: 100,
        },
        {
          label: 'Correlated failure',
          value: Math.round(risk.sub_scores.correlated.score),
          maxValue: 100,
        },
      ]
    : []

  const throttleC =
    (risk?.sub_scores?.thermal?.details?.throttle_threshold_c as number) ?? 95
  const tdpW = (risk?.sub_scores?.power?.details?.tdp_w as number) ?? 100

  const ensureMinPoints = (arr: number[], min = 2) =>
    arr.length >= min ? arr : [...Array(min).fill(arr[0] ?? 0)]

  const tempData = ensureMinPoints(
    history.temperature.map((p) => p.value)
  )
  const utilData = ensureMinPoints(
    history.utilization.map((p) => p.value)
  )
  const powerData = ensureMinPoints(
    history.power.map((p) => p.value)
  )
  const memData = ensureMinPoints(
    history.memory.map((p) => p.value)
  )

  const lastTemp = tempData[tempData.length - 1] ?? 0
  const lastUtil = utilData[utilData.length - 1] ?? 0
  const lastPower = powerData[powerData.length - 1] ?? 0
  const lastMem = memData[memData.length - 1] ?? 0

  const systemInfo = risk
    ? [
        {
          label: 'Composite',
          value: `${risk.composite.score.toFixed(1)} (${risk.composite.severity})`,
        },
        {
          label: 'Trend',
          value: risk.composite.trend,
        },
        {
          label: 'Warning threshold',
          value: `${risk.thresholds.warning}`,
        },
        {
          label: 'Critical threshold',
          value: `${risk.thresholds.critical}`,
        },
      ]
    : []

  const processList =
    processes?.supported && processes.processes.length > 0
      ? processes.processes.map((p) => ({
          name: p.name,
          detail: p.user ? `user: ${p.user}` : '',
          gpuMemory: formatBytes(p.gpu_memory_bytes),
          gpuPercent: `${p.gpu_utilization_pct.toFixed(0)}%`,
          runtime: formatRuntime(p.runtime_seconds),
          isHighUsage: p.gpu_utilization_pct > 50,
        }))
      : []

  return (
    <div className="flex-1 p-6 space-y-4 overflow-auto">
      {/* Back + header */}
      <div className="flex items-center gap-4 mb-4">
        <Link
          to="/"
          className="text-sm text-[#94A3B8] hover:text-[#E8ECF4] transition-colors"
        >
          ← Back
        </Link>
        <h1 className="text-[18px] font-semibold text-[#E8ECF4]">
          Risk Analysis
        </h1>
      </div>

      {/* Composite score + sub-scores */}
      <section className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-6">
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8 items-center">
          <div>
            <h2 className="text-[15px] font-semibold text-[#E8ECF4] mb-1">
              Composite Score
            </h2>
            <LargeHexBadge score={Math.round(score)} severity={hexSeverity} />
          </div>
          <div className="lg:col-span-2">
            <h3 className="text-[13px] font-semibold text-[#E8ECF4] mb-3">
              Sub-scores
            </h3>
            <RiskBreakdown scores={subScores} />
          </div>
        </div>

        {/* 4 sub-score detail cards */}
        {subScores.length > 0 && risk?.sub_scores && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6 pt-6 border-t border-white/[0.06]">
            {[
              { key: 'thermal', label: 'Thermal', data: risk.sub_scores.thermal },
              { key: 'power', label: 'Power', data: risk.sub_scores.power },
              {
                key: 'volatility',
                label: 'Volatility',
                data: risk.sub_scores.volatility,
              },
              {
                key: 'correlated',
                label: 'Correlated',
                data: risk.sub_scores.correlated,
              },
            ].map(({ key, label, data }) => (
              <div
                key={key}
                className="bg-[#0A0C10] rounded-lg border border-white/[0.04] p-4"
              >
                <h4 className="text-[12px] font-semibold text-[#94A3B8] mb-2">
                  {label}
                </h4>
                <p className="text-[15px] font-bold text-[#E8ECF4] mb-2">
                  {data.score.toFixed(1)} (weight: {data.weight})
                </p>
                <div className="space-y-1 text-[11px]">
                  {Object.entries(data.details || {}).map(([k, v]) => (
                    <div
                      key={k}
                      className="flex justify-between text-[#64748B]"
                    >
                      <span>{k.replace(/_/g, ' ')}</span>
                      <span className="text-[#94A3B8]">
                        {typeof v === 'number' ? v.toFixed(2) : String(v)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* 4 telemetry charts */}
      <section className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <TelemetryChart
          title="Temperature"
          currentValue={`${lastTemp.toFixed(0)}°C`}
          valueColor={
            lastTemp >= throttleC * 0.9 ? '#F59E0B' : '#E8ECF4'
          }
          lineColor="#00C9B0"
          data={tempData}
          threshold={{ value: throttleC, label: 'Throttle' }}
          minY={Math.min(...tempData, 0, throttleC - 20) || 0}
          maxY={Math.max(...tempData, throttleC + 10) || 100}
        />
        <TelemetryChart
          title="GPU Utilization"
          currentValue={`${lastUtil.toFixed(0)}%`}
          lineColor="#00C9B0"
          data={utilData}
          minY={0}
          maxY={100}
        />
        <TelemetryChart
          title="Power Draw"
          currentValue={`${lastPower.toFixed(1)}W`}
          lineColor="#00C9B0"
          data={powerData}
          threshold={{ value: tdpW, label: `TDP ${tdpW}W` }}
          minY={0}
          maxY={Math.max(...powerData, tdpW * 1.1) || 150}
        />
        <TelemetryChart
          title="Memory"
          currentValue={`${lastMem.toFixed(1)}%`}
          lineColor="#00C9B0"
          data={memData}
          minY={0}
          maxY={100}
        />
      </section>

      {/* System info + Processes */}
      <section className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <SystemInfo info={systemInfo} />
        {processes?.supported ? (
          <ActiveProcesses processes={processList} />
        ) : (
          <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-5">
            <h3 className="text-[13px] font-semibold text-[#E8ECF4] mb-4">
              Active Processes
            </h3>
            <p className="text-[13px] text-[#64748B]">
              {processes?.note ??
                'Process enumeration is not supported by this adapter.'}
            </p>
          </div>
        )}
      </section>
    </div>
  )
}
