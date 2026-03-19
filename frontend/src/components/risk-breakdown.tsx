interface SubScore {
  label: string
  value: number
  maxValue: number
}

interface RiskBreakdownProps {
  scores: SubScore[]
}

function getColor(value: number): string {
  if (value >= 70) return "#F59E0B"
  if (value >= 50) return "#00C9B0"
  return "#10B981"
}

function getGradient(value: number): string {
  if (value >= 70) {
    return "linear-gradient(90deg, #F59E0B, #EF4444)"
  }
  return ""
}

export function RiskBreakdown({ scores }: RiskBreakdownProps) {
  return (
    <div className="flex flex-col gap-2">
      {scores.map((score, index) => (
        <div key={score.label} className="flex items-center gap-3">
          <span className="text-xs text-[#94A3B8] w-[140px] shrink-0">
            {score.label}
          </span>
          <div className="w-[200px] h-1.5 bg-[#1E293B] rounded-full overflow-hidden">
            <div
              className="h-full rounded-full animate-bar-fill"
              style={{
                width: `${(score.value / score.maxValue) * 100}%`,
                backgroundColor: score.value < 70 ? getColor(score.value) : undefined,
                background: score.value >= 70 ? getGradient(score.value) : undefined,
                animationDelay: `${index * 80}ms`,
              }}
            />
          </div>
          <span
            className="text-[13px] font-semibold w-8 text-right"
            style={{ color: getColor(score.value) }}
          >
            {score.value}
          </span>
        </div>
      ))}
    </div>
  )
}
