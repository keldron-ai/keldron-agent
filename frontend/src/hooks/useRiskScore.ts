import { useState, useEffect } from 'react'

interface SubScore {
  score: number
  weight: number
  weighted_contribution: number
  details: Record<string, unknown>
}

interface RiskBreakdown {
  timestamp: string
  composite: {
    score: number
    severity: 'normal' | 'warning' | 'critical'
    trend: 'stable' | 'rising' | 'falling'
    trend_delta: number
  }
  sub_scores: {
    thermal: SubScore
    power: SubScore
    volatility: SubScore
    correlated: SubScore
  }
  thresholds: { warning: number; critical: number }
}

export function useRiskScore() {
  const [risk, setRisk] = useState<RiskBreakdown | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const fetchRisk = async () => {
      try {
        const res = await fetch('/api/v1/risk')
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        setRisk(await res.json())
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch risk')
      }
    }
    fetchRisk()
  }, [])

  return { risk, error }
}
