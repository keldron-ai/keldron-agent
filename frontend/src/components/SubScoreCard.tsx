import type { ReactNode } from 'react'
import { subScoreColor } from '@/types/severity'

interface SubScoreCardProps {
  name: string
  score: number
  weight: number
  weighted_contribution: number
  details: Record<string, unknown>
  icon: ReactNode
}

function formatKey(key: string): string {
  return key
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

function formatValue(value: unknown): string {
  if (value == null) return '—'
  if (typeof value === 'number') {
    if (Number.isInteger(value)) return String(value)
    return value.toFixed(2)
  }
  return String(value)
}

export function SubScoreCard({
  name,
  score,
  weight,
  weighted_contribution,
  details,
  icon,
}: SubScoreCardProps) {
  const color = subScoreColor(score)

  return (
    <div
      className="rounded-xl border p-4"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <div className="flex items-center gap-2 mb-3">
        <span className="text-[#94A3B8]">{icon}</span>
        <span className="text-sm font-semibold text-[#E8ECF4]">{name}</span>
        <span
          className="w-2 h-2 rounded-full shrink-0"
          style={{ backgroundColor: color }}
        />
      </div>
      <p
        className="text-2xl font-bold mb-1"
        style={{ color }}
      >
        {score.toFixed(1)}
      </p>
      <p className="text-xs text-[#94A3B8] mb-1">× {weight.toFixed(2)}</p>
      <p className="text-sm text-[#E8ECF4] mb-4">
        = {weighted_contribution.toFixed(1)} pts
      </p>
      {Object.keys(details).length > 0 && (
        <div className="space-y-1.5 pt-3 border-t border-white/[0.06]">
          {Object.entries(details).map(([key, value]) => (
            <div
              key={key}
              className="flex justify-between text-xs"
            >
              <span className="text-[#94A3B8]">{formatKey(key)}</span>
              <span className="text-[#E8ECF4]">{formatValue(value)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
