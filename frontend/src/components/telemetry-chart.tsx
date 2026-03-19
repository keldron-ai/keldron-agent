import {
  Line,
  Area,
  ComposedChart,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts'

export interface SparklinePoint {
  timestamp: number
  value: number
}

interface TelemetryChartProps {
  title: string
  data: SparklinePoint[]
  unit: string
  color: string
  thresholdValue?: number
  thresholdLabel?: string
  yDomain?: [number, number]
}

function formatTimeLabel(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

export function TelemetryChart({
  title,
  data,
  unit,
  color,
  thresholdValue,
  thresholdLabel,
  yDomain,
}: TelemetryChartProps) {
  const chartData = data.map((p) => ({
    ...p,
    timeLabel: formatTimeLabel(p.timestamp),
  }))

  const values = chartData.map((d) => d.value)
  const minVal = yDomain?.[0] ?? (values.length ? Math.min(...values, 0) : 0)
  const maxVal = yDomain?.[1] ?? (values.length ? Math.max(...values, 1) : 100)
  const domain: [number, number] = [minVal, maxVal]

  const lastValue = chartData.length > 0 ? chartData[chartData.length - 1].value : null

  return (
    <div
      className="rounded-xl border p-4"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm font-semibold text-[#E8ECF4]">{title}</span>
        {lastValue != null && (
          <span className="text-sm font-semibold text-[#E8ECF4]">
            {lastValue.toFixed(1)}{unit}
          </span>
        )}
      </div>
      <div className="h-[200px] w-full">
        {chartData.length >= 2 ? (
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart
              data={chartData}
              margin={{ top: 8, right: 8, bottom: 8, left: 8 }}
            >
              <CartesianGrid
                strokeDasharray="4 4"
                stroke="rgba(148, 163, 184, 0.1)"
                vertical={false}
              />
              <XAxis
                dataKey="timestamp"
                type="number"
                domain={['dataMin', 'dataMax']}
                tickFormatter={(ts) => formatTimeLabel(ts)}
                tick={{ fill: '#94A3B8', fontSize: 10 }}
                axisLine={{ stroke: 'rgba(148, 163, 184, 0.2)' }}
                tickLine={{ stroke: 'rgba(148, 163, 184, 0.2)' }}
              />
              <YAxis
                domain={domain}
                tickFormatter={(v) => `${v}${unit}`}
                tick={{ fill: '#94A3B8', fontSize: 10 }}
                axisLine={{ stroke: 'rgba(148, 163, 184, 0.2)' }}
                tickLine={{ stroke: 'rgba(148, 163, 184, 0.2)' }}
                width={40}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#0F172A',
                  border: '1px solid rgba(148, 163, 184, 0.2)',
                  borderRadius: '6px',
                }}
                labelStyle={{ color: '#94A3B8' }}
                formatter={(val: number) => [`${val?.toFixed(1) ?? ''} ${unit}`]}
                labelFormatter={(ts) =>
                  ts ? new Date(ts).toLocaleTimeString() : ''
                }
              />
              {thresholdValue != null && (
                <ReferenceLine
                  y={thresholdValue}
                  stroke="#F59E0B"
                  strokeDasharray="6 4"
                  strokeWidth={1.5}
                  label={{
                    value: thresholdLabel ?? '',
                    position: 'right',
                    fill: '#F59E0B',
                    fontSize: 10,
                  }}
                />
              )}
              <Area
                type="monotone"
                dataKey="value"
                fill={color}
                fillOpacity={0.1}
                stroke="none"
              />
              <Line
                type="monotone"
                dataKey="value"
                stroke={color}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
            </ComposedChart>
          </ResponsiveContainer>
        ) : (
          <div className="h-full flex items-center justify-center text-[#64748B] text-sm">
            —
          </div>
        )}
      </div>
    </div>
  )
}
