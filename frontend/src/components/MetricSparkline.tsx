import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
} from 'recharts'

export interface SparklinePoint {
  timestamp: number
  value: number
}

type Severity = 'normal' | 'warning' | 'critical'

interface MetricSparklineProps {
  label: string
  value: number
  unit: string
  history: SparklinePoint[]
  thresholdValue?: number
  thresholdLabel?: string
  severity?: Severity
  min?: number
  max?: number
}

const FIVE_MIN_MS = 5 * 60 * 1000

function computeTrend(history: SparklinePoint[]): 'stable' | 'rising' | 'falling' {
  if (history.length < 2) return 'stable'
  const now = Date.now()
  const cutoff = now - FIVE_MIN_MS
  const recent = history.filter((p) => p.timestamp > cutoff)
  if (recent.length < 2) return 'stable'
  const first = recent[0].value
  const last = recent[recent.length - 1].value
  const diff = last - first
  if (diff > 1) return 'rising'
  if (diff < -1) return 'falling'
  return 'stable'
}

function TrendArrow({ trend }: { trend: 'stable' | 'rising' | 'falling' }) {
  const severityColor = '#F59E0B'
  switch (trend) {
    case 'rising':
      return <span style={{ color: severityColor }}>▲</span>
    case 'falling':
      return <span className="text-[#22C55E]">▼</span>
    default:
      return <span className="text-[#94A3B8]">—</span>
  }
}

export function MetricSparkline({
  label,
  value,
  unit,
  history,
  thresholdValue,
  thresholdLabel,
  severity,
  min,
  max,
}: MetricSparklineProps) {
  const trend = computeTrend(history)
  const chartData = history.map((p) => ({
    ...p,
    time: new Date(p.timestamp).toISOString(),
  }))

  const yMin = min ?? (chartData.length ? Math.min(...chartData.map((d) => d.value), 0) : 0)
  const yMax = max ?? (chartData.length ? Math.max(...chartData.map((d) => d.value), 1) : 100)
  const domain = [yMin, yMax]

  return (
    <div
      className="rounded-xl border p-4 flex flex-col gap-2"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <span className="text-xs text-[#94A3B8]">{label}</span>
      <div className="flex items-center gap-2">
        <span className="text-2xl font-bold text-[#E8ECF4]">
          {value.toFixed(1)}
        </span>
        <span className="text-sm text-[#94A3B8]">{unit}</span>
        <TrendArrow trend={trend} />
      </div>
      <div className="h-[60px] w-full">
        {chartData.length >= 2 ? (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart
              data={chartData}
              margin={{ top: 2, right: 2, bottom: 2, left: 2 }}
            >
              <defs>
                <linearGradient id="sparkline-fill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#00C9B0" stopOpacity={0.2} />
                  <stop offset="100%" stopColor="#00C9B0" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="timestamp" hide />
              <YAxis domain={domain} hide />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#0F172A',
                  border: '1px solid rgba(148, 163, 184, 0.2)',
                  borderRadius: '6px',
                }}
                labelStyle={{ color: '#94A3B8' }}
                formatter={(val: number) => [`${val?.toFixed(1) ?? ''} ${unit}`]}
                labelFormatter={(label) =>
                  label ? new Date(label).toLocaleTimeString() : ''
                }
              />
              {thresholdValue != null && (
                <ReferenceLine
                  y={thresholdValue}
                  stroke="#F59E0B"
                  strokeDasharray="4 4"
                  strokeWidth={1}
                />
              )}
              <Area
                type="monotone"
                dataKey="value"
                stroke="#00C9B0"
                strokeWidth={1.5}
                fill="url(#sparkline-fill)"
              />
            </AreaChart>
          </ResponsiveContainer>
        ) : (
          <div className="h-full flex items-center text-[#64748B] text-xs">
            —
          </div>
        )}
      </div>
    </div>
  )
}
