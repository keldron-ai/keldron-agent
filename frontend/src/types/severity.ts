/** Composite risk severity bands (matches agent /api/v1/risk). */
export type RiskSeverityBand =
  | 'normal'
  | 'active'
  | 'elevated'
  | 'warning'
  | 'critical'

/** Sub-score dot color by raw score (aligned with composite bands). */
export function subScoreColor(score: number): string {
  if (score < 30) return '#00C9B0'
  if (score < 50) return '#3B82F6'
  if (score < 70) return '#F5A623'
  if (score < 90) return '#FF6B35'
  return '#FF3B3B'
}
