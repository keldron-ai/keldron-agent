import { Link } from 'react-router-dom'
import { Thermometer, Zap, Activity, HardDrive } from 'lucide-react'
import { useTelemetry } from '@/context/TelemetryContext'
import { RiskHexBadge } from '@/components/RiskHexBadge'
import { SubScoreBars } from '@/components/SubScoreBars'
import { SubScoreCard } from '@/components/SubScoreCard'
import { TelemetryChart } from '@/components/telemetry-chart'
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

const SUB_SCORE_CONFIG = [
  {
    key: 'thermal' as const,
    name: 'Thermal',
    icon: <Thermometer className="w-4 h-4" />,
  },
  {
    key: 'power' as const,
    name: 'Power',
    icon: <Zap className="w-4 h-4" />,
  },
  {
    key: 'volatility' as const,
    name: 'Volatility',
    icon: <Activity className="w-4 h-4" />,
  },
  {
    key: 'memory' as const,
    name: 'Memory',
    icon: <HardDrive className="w-4 h-4" />,
  },
] as const

export function RiskDrilldown() {
  const { risk, history, processes } = useTelemetry()

  const score = risk?.composite?.score ?? 0
  const severity = risk?.composite?.severity
  const trend = risk?.composite?.trend ?? 'stable'
  const trendDelta = risk?.composite?.trend_delta ?? 0
  const hexSeverity = mapApiSeverity(severity, score)

  const subScores = risk?.sub_scores
  const throttleC = (subScores?.thermal?.details?.throttle_threshold_c as number) ?? 95
  const tdpW = (subScores?.power?.details?.tdp_w as number) ?? 100

  return (
    <div className="flex-1 p-6 space-y-6 overflow-auto">
      {/* Back + header */}
      <div className="flex items-center gap-4">
        <Link
          to="/"
          className="text-sm text-[#94A3B8] hover:text-[#E8ECF4] transition-colors"
        >
          ← Back to Dashboard
        </Link>
        <h1 className="text-lg font-semibold text-[#E8ECF4]">
          Risk Analysis
        </h1>
      </div>

      {/* Composite score + sub-score bars */}
      <section
        className="rounded-xl border p-6"
        style={{
          backgroundColor: '#0F172A',
          borderColor: 'rgba(148, 163, 184, 0.1)',
        }}
      >
        <div className="flex flex-col lg:flex-row gap-8 items-start">
          <div className="flex flex-col items-center gap-4">
            <RiskHexBadge
              score={score}
              severity={hexSeverity}
              trend={trend}
              size="md"
            />
            <div className="text-center space-y-1">
              <p className="text-sm text-[#94A3B8]">
                Composite: <span className="text-[#E8ECF4] font-medium">{score >= 10 ? score.toFixed(0) : score.toFixed(1)}</span>
              </p>
              <p className="text-sm text-[#94A3B8]">
                Severity: <span style={{ color: hexSeverity === 'normal' ? '#22C55E' : hexSeverity === 'warning' ? '#F59E0B' : '#EF4444' }}>{severity ?? '—'}</span>
              </p>
              <p className="text-sm text-[#94A3B8]">
                Trend: {trend} ({trendDelta >= 0 ? '+' : ''}{trendDelta.toFixed(1)})
              </p>
            </div>
          </div>
          <div className="flex-1 w-full">
            <SubScoreBars subScores={subScores} />
          </div>
        </div>

        {/* 4 sub-score cards */}
        {subScores && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6 pt-6 border-t border-white/[0.06]">
            {SUB_SCORE_CONFIG.map(({ key, name, icon }) => {
              const data = subScores[key]
              if (!data) return null
              return (
                <SubScoreCard
                  key={key}
                  name={name}
                  score={data.score}
                  weight={data.weight}
                  weighted_contribution={data.weighted_contribution}
                  details={data.details ?? {}}
                  icon={icon}
                />
              )
            })}
          </div>
        )}
      </section>

      {/* 4 telemetry charts */}
      <section className="space-y-4">
        <TelemetryChart
          title="Temperature (30 min)"
          data={history.temperature}
          unit="°C"
          color="#00C9B0"
          thresholdValue={throttleC}
          thresholdLabel="Throttle"
          yDomain={
            history.temperature.length > 0
              ? [
                  Math.min(
                    Math.min(...history.temperature.map((p) => p.value)),
                    throttleC - 20,
                    0
                  ),
                  Math.max(
                    Math.max(...history.temperature.map((p) => p.value)),
                    throttleC + 10,
                    100
                  ),
                ]
              : [0, 100]
          }
        />
        <TelemetryChart
          title="GPU Utilization (30 min)"
          data={history.utilization}
          unit="%"
          color="#3B82F6"
          yDomain={[0, 100]}
        />
        <TelemetryChart
          title="Power Draw (30 min)"
          data={history.power}
          unit="W"
          color="#F59E0B"
          thresholdValue={tdpW}
          thresholdLabel={`TDP ${tdpW}W`}
          yDomain={
            history.power.length > 0
              ? [
                  0,
                  Math.max(
                    Math.max(...history.power.map((p) => p.value)),
                    tdpW * 1.1,
                    150
                  ),
                ]
              : [0, 150]
          }
        />
        <TelemetryChart
          title="Memory Usage (30 min)"
          data={history.memory}
          unit="%"
          color="#00E5CC"
          yDomain={[0, 100]}
        />
      </section>

      {/* Process table */}
      <section>
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
      </section>
    </div>
  )
}
