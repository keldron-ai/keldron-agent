import { useState, useId, useEffect, useMemo } from "react"

interface TelemetryChartProps {
  title: string
  currentValue: string
  valueColor?: string
  lineColor: string
  data: number[]
  threshold?: {
    value: number
    label: string
  }
  minY?: number
  maxY?: number
  eventLabel?: string
}

const TIME_RANGES = ["30m", "1H", "6H", "24H"] as const

export function TelemetryChart({
  title,
  currentValue,
  valueColor = "#E8ECF4",
  lineColor,
  data,
  threshold,
  minY = 0,
  maxY = 100,
  eventLabel,
}: TelemetryChartProps) {
  const [activeRange, setActiveRange] = useState<string>("30m")
  const [mounted, setMounted] = useState(false)
  const uniqueId = useId()

  useEffect(() => {
    setMounted(true)
  }, [])

  // Slice data based on selected range (data assumed to cover 30m window)
  const filteredData = useMemo(() => {
    if (data.length === 0) return data
    const ratios: Record<string, number> = { "30m": 1, "1H": 1, "6H": 1 }
    const ratio = ratios[activeRange] ?? 1
    const start = Math.floor(data.length * (1 - ratio))
    return data.slice(start)
  }, [data, activeRange])

  const width = 400
  const height = 160
  const padding = { top: 10, right: 50, bottom: 10, left: 10 }
  const chartWidth = width - padding.left - padding.right
  const chartHeight = height - padding.top - padding.bottom

  const normalizeY = (value: number) => {
    if (maxY === minY) return padding.top + chartHeight / 2
    const normalized = (value - minY) / (maxY - minY)
    return padding.top + chartHeight * (1 - normalized)
  }

  const safeDenom = Math.max(1, filteredData.length - 1)

  const points = filteredData.length === 0
    ? ""
    : filteredData
        .map((value, index) => {
          const x = padding.left + (index / safeDenom) * chartWidth
          const y = normalizeY(value)
          return `${x},${y}`
        })
        .join(" ")

  const areaPath = filteredData.length === 0
    ? ""
    : `
    M ${padding.left},${normalizeY(filteredData[0])}
    ${filteredData
      .map((value, index) => {
        const x = padding.left + (index / safeDenom) * chartWidth
        const y = normalizeY(value)
        return `L ${x},${y}`
      })
      .join(" ")}
    L ${padding.left + (filteredData.length - 1) / safeDenom * chartWidth},${padding.top + chartHeight}
    L ${padding.left},${padding.top + chartHeight}
    Z
  `

  const thresholdY = threshold ? normalizeY(threshold.value) : null

  // Calculate approximate line length for dash animation
  const lineLength = chartWidth * 1.5

  // Event label position (near right edge at current value)
  const lastDataPoint = filteredData.length > 0 ? filteredData[filteredData.length - 1] : null
  const eventLabelX = padding.left + chartWidth - 60
  const eventLabelY = lastDataPoint !== null ? normalizeY(lastDataPoint) - 12 : 0

  const gradientId = `gradient-${uniqueId}`
  const fadeGradientId = `fade-${uniqueId}`

  return (
    <div className="bg-[#0F172A] rounded-xl border border-white/[0.06] p-4 hover:border-white/[0.12] transition-colors">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <span className="text-[13px] font-semibold text-[#E8ECF4]">{title}</span>
          <span className="text-[13px] font-semibold" style={{ color: valueColor }}>
            {currentValue}
          </span>
        </div>
        <div className="flex items-center gap-1">
          {TIME_RANGES.map((range) => (
            <button
              key={range}
              disabled={range === "24H"}
              onClick={() => setActiveRange(range)}
              className={`
                px-2 py-0.5 rounded text-[11px] transition-colors
                ${
                  activeRange === range
                    ? "bg-[rgba(0,201,176,0.15)] text-[#00C9B0]"
                    : "text-[#64748B] hover:text-[#94A3B8]"
                }
                disabled:cursor-not-allowed disabled:opacity-50
              `}
            >
              <span className="flex items-center gap-1">
                {range}
                {range === "24H" && (
                  <svg
                    width="10"
                    height="10"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="#475569"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                    <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                  </svg>
                )}
              </span>
            </button>
          ))}
        </div>
      </div>

      {/* Chart */}
      <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
        <defs>
          {/* Area fill gradient */}
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={lineColor} stopOpacity="0.15" />
            <stop offset="100%" stopColor={lineColor} stopOpacity="0" />
          </linearGradient>
          {/* Line fade gradient (older = faded, newer = bright) */}
          <linearGradient id={fadeGradientId} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor={lineColor} stopOpacity="0.4" />
            <stop offset="100%" stopColor={lineColor} stopOpacity="1" />
          </linearGradient>
        </defs>

        {/* Grid lines */}
        {[0.25, 0.5, 0.75].map((ratio) => (
          <line
            key={ratio}
            x1={padding.left}
            y1={padding.top + chartHeight * ratio}
            x2={padding.left + chartWidth}
            y2={padding.top + chartHeight * ratio}
            stroke="rgba(255,255,255,0.04)"
            strokeDasharray="4 4"
          />
        ))}

        {/* Threshold line */}
        {thresholdY !== null && (
          <>
            <line
              x1={padding.left}
              y1={thresholdY}
              x2={padding.left + chartWidth}
              y2={thresholdY}
              stroke="rgba(239,68,68,0.4)"
              strokeDasharray="6 4"
            />
            <text
              x={padding.left + chartWidth + 4}
              y={thresholdY + 4}
              fill="#EF4444"
              fontSize="10"
              fontFamily="system-ui, sans-serif"
            >
              {threshold?.label}
            </text>
          </>
        )}

        {/* Area fill */}
        <path d={areaPath} fill={`url(#${gradientId})`} />

        {/* Line with draw animation */}
        <polyline
          points={points}
          fill="none"
          stroke={`url(#${fadeGradientId})`}
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeDasharray={lineLength}
          strokeDashoffset={mounted ? 0 : lineLength}
          style={{
            transition: "stroke-dashoffset 600ms ease-out",
          }}
        />

        {/* Event label */}
        {eventLabel && lastDataPoint !== null && (
          <g>
            <rect
              x={eventLabelX - 4}
              y={eventLabelY - 8}
              width={eventLabel.length * 6 + 12}
              height={16}
              rx="4"
              fill="rgba(245,158,11,0.15)"
            />
            <text
              x={eventLabelX + 2}
              y={eventLabelY + 4}
              fill="#F59E0B"
              fontSize="9"
              fontFamily="system-ui, sans-serif"
              fontWeight="500"
            >
              {eventLabel}
            </text>
          </g>
        )}
      </svg>
    </div>
  )
}
