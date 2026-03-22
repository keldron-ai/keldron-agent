export interface SubScore {
  score: number
  weight: number
  weighted_contribution: number
  details: Record<string, unknown>
}

export interface SubScores {
  thermal: SubScore
  power: SubScore
  volatility: SubScore
  memory: SubScore
}
