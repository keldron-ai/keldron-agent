interface SparklinePoint {
  timestamp: number
  value: number
}

interface DataSparklineProps {
  data: SparklinePoint[]
  currentValue: string
  label: string
  trend: 'stable' | 'rising' | 'falling'
  warning?: boolean
  lineColor?: string
}

function TrendArrow({ trend }: { trend: 'stable' | 'rising' | 'falling' }) {
  switch (trend) {
    case 'rising':
      return <span className="text-[#F59E0B]">▲</span>
    case 'falling':
      return <span className="text-[#10B981]">▼</span>
    default:
      return <span className="text-[#64748B]">—</span>
  }
}

export function DataSparkline({
  data,
  currentValue,
  label,
  trend,
  warning,
  lineColor = '#00C9B0',
}: DataSparklineProps) {
  const values = data.map((p) => p.value)
  const strokeColor = warning ? '#F59E0B' : lineColor
  const min = values.length ? Math.min(...values, 0) : 0
  const max = values.length ? Math.max(...values, 1) : 1
  const range = max - min || 1
  const width = 80
  const height = 18

  const points = values
    .map((v, i) => {
      const x = (i / (values.length - 1 || 1)) * width
      const y = height - ((v - min) / range) * (height - 2) - 1
      return `${x},${y}`
    })
    .join(' ')

  return (
    <div className="flex items-center h-7">
      <span className="text-xs text-[#64748B] w-[100px] shrink-0">{label}</span>
      <div className="flex-1 px-2 flex items-center justify-center">
        {values.length > 1 ? (
          <svg
            width={width}
            height={height}
            viewBox={`0 0 ${width} ${height}`}
            preserveAspectRatio="none"
            className="block"
          >
            <polyline
              points={points}
              fill="none"
              stroke={strokeColor}
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        ) : (
          <span className="text-[11px] text-[#64748B]">—</span>
        )}
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <span
          className={`text-[13px] font-semibold ${
            warning ? 'text-[#F59E0B]' : 'text-[#E8ECF4]'
          }`}
        >
          {currentValue}
        </span>
        <TrendArrow trend={trend} />
      </div>
    </div>
  )
}
