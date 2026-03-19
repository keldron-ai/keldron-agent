interface SubScore {
  score: number
  weight: number
  weighted_contribution: number
  details: Record<string, unknown>
}

interface SubScores {
  thermal: SubScore
  power: SubScore
  volatility: SubScore
  correlated: SubScore
}

interface SubScoreBarsProps {
  subScores: SubScores | null | undefined
}

const LABELS: Record<keyof SubScores, string> = {
  thermal: 'Thermal margin',
  power: 'Power headroom',
  volatility: 'Load volatility',
  correlated: 'Correlated failure',
}

function getBarColor(score: number): string {
  if (score < 40) return '#22C55E'
  if (score < 70) return '#F59E0B'
  return '#EF4444'
}

export function SubScoreBars({ subScores }: SubScoreBarsProps) {
  if (!subScores) return null

  const entries: { key: keyof SubScores; score: number }[] = [
    { key: 'thermal', score: subScores.thermal?.score ?? 0 },
    { key: 'power', score: subScores.power?.score ?? 0 },
    { key: 'volatility', score: subScores.volatility?.score ?? 0 },
    { key: 'correlated', score: subScores.correlated?.score ?? 0 },
  ]

  return (
    <div className="space-y-3">
      {entries.map(({ key, score }) => {
        const color = getBarColor(score)
        return (
          <div key={key} className="flex items-center gap-3">
            <span className="text-xs text-[#94A3B8] w-28 shrink-0">
              {LABELS[key]}
            </span>
            <div className="flex-1 h-2 rounded-full bg-white/10 overflow-hidden">
              <div
                className="h-full rounded-full transition-all"
                style={{
                  width: `${Math.min(100, Math.max(0, score))}%`,
                  backgroundColor: color,
                }}
              />
            </div>
            <span className="text-sm font-medium text-[#E8ECF4] w-8 text-right">
              {score.toFixed(0)}
            </span>
          </div>
        )
      })}
    </div>
  )
}
