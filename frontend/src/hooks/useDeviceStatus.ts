import { useState, useEffect } from 'react'

interface DeviceStatus {
  device: {
    hostname: string
    adapter: string
    hardware: string
    behavior_class: string
    os: string
    arch: string
    uptime_seconds: number
  }
  telemetry: {
    timestamp: string
    temperature_c: number
    gpu_utilization_pct: number
    power_draw_w: number
    memory_used_pct: number
    memory_used_bytes: number
    memory_total_bytes: number
    thermal_state: string
    throttle_active: boolean
    fan_rpm: number | null
    neural_engine_util_pct: number | null
  }
  risk: {
    composite_score: number
    severity: 'normal' | 'warning' | 'critical'
    trend: 'stable' | 'rising' | 'falling'
    trend_delta: number
  }
  agent: {
    version: string
    poll_interval_s: number
    adapters_active: string[]
    cloud_connected: boolean
  }
  health: {
    thermal_dynamic_range?: {
      available: boolean
      tdr_celsius: number | null
      idle_temp_c: number | null
      peak_temp_c: number | null
      rating: string | null
      idle_sample_count: number
      peak_sample_count: number
      window_hours: number
    }
    thermal_recovery?: {
      available: boolean
      last_recovery_seconds: number | null
      last_peak_temp_c: number | null
      last_baseline_temp_c: number | null
      rating: string | null
      recovery_count: number
      session_avg_seconds: number | null
    }
    perf_per_watt?: {
      available: boolean
      value: number | null
      unit: string
    }
    thermal_stability?: {
      available: boolean
      std_dev_celsius: number | null
      rating: string | null
      under_sustained_load: boolean
      sample_count: number
      window_minutes: number
    }
  } | null
}

export function useDeviceStatus() {
  const [status, setStatus] = useState<DeviceStatus | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchStatus = async () => {
      try {
        const res = await fetch('/api/v1/status')
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = await res.json()
        setStatus(data)
        setError(null)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch status')
      } finally {
        setLoading(false)
      }
    }
    fetchStatus()
  }, [])

  return { status, error, loading }
}
