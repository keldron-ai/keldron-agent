export interface StatusHealth {
  warming_up?: boolean
  thermal_dynamic_range?: {
    available: boolean
    no_sustained_load?: boolean
    warming_up?: boolean
    avg_temp_c?: number | null
    max_temp_c?: number | null
    headroom_used_pct?: number | null
    peak_proximity_pct?: number | null
    rating: string | null
    note?: string | null
  }
  thermal_recovery?: {
    available: boolean
    no_spikes?: boolean
    spike_active?: boolean
    active_spike_seconds?: number | null
    last_recovery_seconds?: number | null
    rating: string | null
    note?: string | null
    warming_up?: boolean
  }
  perf_per_watt?: {
    available: boolean
    value: number | null
    unit: string
  }
  thermal_stability?: {
    available: boolean
    std_dev_celsius?: number | null
    rating: string | null
    warming_up?: boolean
    note?: string | null
  }
}
