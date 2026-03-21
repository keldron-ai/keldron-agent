import { useId } from 'react'
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

import type { SparklinePoint } from '@/types/sparkline'
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion'

export interface ChartEventFlash {
  text: string
  key: number
}

interface TelemetryChartProps {
  title: string
  data: SparklinePoint[]
  unit: string
  color: string
  thresholdValue?: number
  thresholdLabel?: string
  thresholdStrokeColor?: string
  yDomain?: [number, number]
  currentValue?: number
  currentValueSeverity?: 'normal' | 'warning' | 'critical'
  showHighTempBadge?: boolean
  eventFlash?: ChartEventFlash | null
  onEventFlashEnd?: () => void
  /** Chart plot area height (Tailwind class). Default 200px for full-page charts. */
  chartHeightClassName?: string
  /** Smaller header + padding for dense dashboard grid. */
  compactLayout?: boolean
}

function formatTimeLabel(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatYAxisTick(v: number | string, unit: string): string {
  const n = typeof v === 'number' ? v : Number(v)
  if (!Number.isFinite(n)) return `—${unit}`
  return `${Math.round(n)}${unit}`
}

/** Header value: integer when ~whole, one decimal when there is a meaningful fraction. */
function formatHeaderValue(v: number): string {
  const rounded = Math.round(v)
  if (Math.abs(v - rounded) < 0.05) return String(rounded)
  return v.toFixed(1)
}

function formatTooltipValue(val: number): string {
  if (!Number.isFinite(val)) return '—'
  return val.toFixed(1)
}

export function TelemetryChart({
  title,
  data,
  unit,
  color,
  thresholdValue,
  thresholdLabel,
  thresholdStrokeColor = '#F59E0B',
  yDomain,
  currentValue,
  currentValueSeverity = 'normal',
  showHighTempBadge = false,
  eventFlash,
  onEventFlashEnd,
  chartHeightClassName = 'h-[200px]',
  compactLayout = false,
}: TelemetryChartProps) {
  const rawId = useId().replace(/:/g, '')
  const strokeGradId = `tc-stroke-${rawId}`
  const fillGradId = `tc-fill-${rawId}`
  const reducedMotion = usePrefersReducedMotion()

  const chartData = data.map((p) => ({
    ...p,
    timeLabel: formatTimeLabel(p.timestamp),
  }))

  const values = chartData.map((d) => d.value)
  const minVal = yDomain?.[0] ?? (values.length ? Math.min(...values, 0) : 0)
  const maxVal = yDomain?.[1] ?? (values.length ? Math.max(...values, 1) : 100)
  const domain: [number, number] = [minVal, maxVal]

  const displayValue =
    currentValue ?? (chartData.length > 0 ? chartData[chartData.length - 1].value : null)
  const valueColor =
    currentValueSeverity === 'critical'
      ? '#EF4444'
      : currentValueSeverity === 'warning'
        ? '#F59E0B'
        : '#00C9B0'

  const lineStroke = reducedMotion ? color : `url(#${strokeGradId})`
  const areaFill = reducedMotion ? color : `url(#${fillGradId})`
  const areaFillOpacity = reducedMotion ? 0.1 : 1

  const headerTitleClass = compactLayout
    ? 'text-[9px] font-bold uppercase tracking-wider text-[#94A3B8]'
    : 'text-sm font-semibold text-[#E8ECF4]'

  return (
    <div
      className={`rounded-xl border ${compactLayout ? 'p-1.5' : 'p-4'}`}
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <div
        className={`flex items-center justify-between gap-2 ${compactLayout ? 'mb-0.5' : 'mb-3'}`}
      >
        <div className="flex items-center gap-2 flex-wrap min-w-0">
          <span className={headerTitleClass}>{title}</span>
          {displayValue != null && (
            <span className="text-sm font-semibold" style={{ color: valueColor }}>
              {formatHeaderValue(displayValue)}
              {unit}
            </span>
          )}
          {showHighTempBadge && (
            <span
              className="text-[10px] font-medium px-2 py-0.5 rounded-full"
              style={{
                backgroundColor: 'rgba(245, 158, 11, 0.2)',
                color: '#F59E0B',
              }}
            >
              [HIGH TEMP]
            </span>
          )}
          {eventFlash && !reducedMotion && (
            <span
              key={eventFlash.key}
              className="text-[10px] font-medium px-2 py-0.5 rounded-full animate-chart-event-label"
              style={{
                backgroundColor: 'rgba(245, 158, 11, 0.2)',
                color: '#F59E0B',
              }}
              onAnimationEnd={(e) => {
                const name = (e as AnimationEvent).animationName
                if (name.includes('chart-event-label')) {
                  onEventFlashEnd?.()
                }
              }}
            >
              {eventFlash.text}
            </span>
          )}
        </div>
      </div>
      <div className={`${chartHeightClassName} w-full`}>
        {chartData.length >= 2 ? (
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart
              data={chartData}
              margin={{ top: 8, right: 8, bottom: 8, left: 8 }}
            >
              {!reducedMotion && (
                <defs>
                  <linearGradient id={strokeGradId} x1="0" y1="0" x2="1" y2="0">
                    <stop offset="0%" stopColor={color} stopOpacity={0.2} />
                    <stop offset="100%" stopColor={color} stopOpacity={1} />
                  </linearGradient>
                  <linearGradient id={fillGradId} x1="0" y1="0" x2="1" y2="0">
                    <stop offset="0%" stopColor={color} stopOpacity={0.05} />
                    <stop offset="100%" stopColor={color} stopOpacity={0.15} />
                  </linearGradient>
                </defs>
              )}
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
                allowDecimals={false}
                tickCount={6}
                tickFormatter={(v) => formatYAxisTick(v, unit)}
                tick={{ fill: '#94A3B8', fontSize: 10 }}
                axisLine={{ stroke: 'rgba(148, 163, 184, 0.2)' }}
                tickLine={{ stroke: 'rgba(148, 163, 184, 0.2)' }}
                width={44}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#0F172A',
                  border: '1px solid rgba(148, 163, 184, 0.2)',
                  borderRadius: '6px',
                }}
                labelStyle={{ color: '#94A3B8' }}
                formatter={(val: number) => [
                  `${Number.isFinite(val) ? formatTooltipValue(val) : '—'}${unit}`,
                ]}
                labelFormatter={(ts) =>
                  ts ? new Date(ts).toLocaleTimeString() : ''
                }
              />
              {thresholdValue != null && (
                <ReferenceLine
                  y={thresholdValue}
                  stroke={thresholdStrokeColor}
                  strokeDasharray="6 4"
                  strokeWidth={1.5}
                  label={{
                    value: thresholdLabel ?? '',
                    position: 'right',
                    fill: thresholdStrokeColor,
                    fontSize: 10,
                  }}
                />
              )}
              <Area
                type="monotone"
                dataKey="value"
                fill={areaFill}
                fillOpacity={areaFillOpacity}
                stroke="none"
              />
              <Line
                type="monotone"
                dataKey="value"
                stroke={lineStroke}
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
