import React, {
  createContext,
  useContext,
  useState,
  useEffect,
  useRef,
  useCallback,
} from 'react'

import type { SparklinePoint } from '@/types/sparkline'
import type { RiskSeverityBand } from '@/types/severity'

interface TelemetryUpdate {
  type: 'telemetry_update'
  timestamp: string
  telemetry: {
    temperature_c: number
    gpu_utilization_pct: number
    power_draw_w: number
    memory_used_pct: number
    thermal_state: string
    throttle_active: boolean
  }
  risk: {
    composite_score: number
    severity: RiskSeverityBand
    trend: 'stable' | 'rising' | 'falling'
  }
  health?: {
    tdr_celsius: number | null
    tdr_rating: string | null
    stability_celsius: number | null
    perf_per_watt: number | null
  }
}

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
    fan_rpm?: number | null
    neural_engine_util_pct?: number | null
  }
  risk: {
    composite_score: number
    severity: RiskSeverityBand
    trend: 'stable' | 'rising' | 'falling'
    trend_delta: number
  }
  agent: {
    version: string
    poll_interval_s: number
    adapters_active: string[]
    cloud_connected: boolean
  }
  health: Record<string, unknown> | null
}

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
    severity: RiskSeverityBand
    trend: 'stable' | 'rising' | 'falling'
    trend_delta: number
  }
  sub_scores: {
    thermal: SubScore
    power: SubScore
    volatility: SubScore
    memory: SubScore
  }
  thresholds: {
    active: number
    elevated: number
    warning: number
    critical: number
  }
}

interface ProcessList {
  timestamp: string
  processes: Array<{
    pid: number
    name: string
    gpu_memory_bytes: number
    gpu_utilization_pct: number
    runtime_seconds: number
    user: string
  }>
  supported: boolean
  note: string | null
}

/** Go ParseDuration strings used by /api/v1/history */
const HISTORY_WINDOW_MS: Record<string, number> = {
  '30m': 30 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
}

interface TelemetryContextValue {
  connected: boolean
  latest: TelemetryUpdate | null
  status: DeviceStatus | null
  risk: RiskBreakdown | null
  processes: ProcessList | null
  history: {
    temperature: SparklinePoint[]
    utilization: SparklinePoint[]
    power: SparklinePoint[]
    memory: SparklinePoint[]
    risk: SparklinePoint[]
  }
  statusLoading: boolean
  statusError: string | null
  /** Refetch history for the given window and align live trim (e.g. "30m", "1h", "6h", "24h"). */
  refreshHistory: (windowQuery: string, signal?: AbortSignal) => Promise<void>
}

const TelemetryContext = createContext<TelemetryContextValue | null>(null)

function toPoints(
  arr: Array<Record<string, unknown>>,
  key: string
): SparklinePoint[] {
  return arr.map((p) => {
    const parsed = Date.parse(p.timestamp as string)
    const timestamp = Number.isNaN(parsed) ? Date.now() : parsed
    const raw = Number(p[key])
    const value = Number.isNaN(raw) ? 0 : raw
    return { timestamp, value }
  })
}

export function TelemetryProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<DeviceStatus | null>(null)
  const [statusLoading, setStatusLoading] = useState(true)
  const [statusError, setStatusError] = useState<string | null>(null)
  const [risk, setRisk] = useState<RiskBreakdown | null>(null)
  const [processes, setProcesses] = useState<ProcessList | null>(null)
  const [connected, setConnected] = useState(false)
  const [latest, setLatest] = useState<TelemetryUpdate | null>(null)

  const [tempHistory, setTempHistory] = useState<SparklinePoint[]>([])
  const [utilHistory, setUtilHistory] = useState<SparklinePoint[]>([])
  const [powerHistory, setPowerHistory] = useState<SparklinePoint[]>([])
  const [memoryHistory, setMemoryHistory] = useState<SparklinePoint[]>([])
  const [riskHistory, setRiskHistory] = useState<SparklinePoint[]>([])

  const historyWindowMsRef = useRef(HISTORY_WINDOW_MS['30m'])

  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>()
  const shouldReconnectRef = useRef(true)

  const addPoint = useCallback(
    (
      setter: React.Dispatch<React.SetStateAction<SparklinePoint[]>>,
      value: number
    ) => {
      const now = Date.now()
      const windowMs = historyWindowMsRef.current
      setter((prev) => {
        const cutoff = now - windowMs
        const filtered = prev.filter((p) => p.timestamp > cutoff)
        return [...filtered, { timestamp: now, value }]
      })
    },
    []
  )

  const refreshHistory = useCallback(async (windowQuery: string, signal?: AbortSignal) => {
    const ms = HISTORY_WINDOW_MS[windowQuery] ?? HISTORY_WINDOW_MS['30m']
    try {
      const res = await fetch(
        `/api/v1/history?window=${encodeURIComponent(windowQuery)}`,
        signal ? { signal } : undefined
      )
      if (!res.ok) return
      const data = await res.json()
      historyWindowMsRef.current = ms
      const points = data.points as Array<Record<string, unknown>> | undefined
      if (points && points.length > 0) {
        setTempHistory(toPoints(points, 'temperature_c'))
        setUtilHistory(toPoints(points, 'gpu_utilization_pct'))
        setPowerHistory(toPoints(points, 'power_draw_w'))
        setMemoryHistory(toPoints(points, 'memory_used_pct'))
        setRiskHistory(toPoints(points, 'composite_score'))
      } else {
        setTempHistory([])
        setUtilHistory([])
        setPowerHistory([])
        setMemoryHistory([])
        setRiskHistory([])
      }
    } catch {
      /* History endpoint may not exist on older agents — silent fail */
    }
  }, [])

  useEffect(() => {
    const fetchStatus = async () => {
      try {
        const res = await fetch('/api/v1/status')
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = await res.json()
        setStatus(data)
        setStatusError(null)
      } catch (err) {
        setStatusError(
          err instanceof Error ? err.message : 'Failed to fetch'
        )
      } finally {
        setStatusLoading(false)
      }
    }
    fetchStatus()
    const interval = setInterval(fetchStatus, 30000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    const fetchRisk = async () => {
      try {
        const res = await fetch('/api/v1/risk')
        if (!res.ok) return
        setRisk(await res.json())
      } catch {
        /* silent */
      }
    }
    fetchRisk()
    const interval = setInterval(fetchRisk, 30000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    const fetchProcesses = async () => {
      try {
        const res = await fetch('/api/v1/processes')
        let data: unknown
        try {
          data = await res.json()
        } catch {
          return
        }
        if (
          data &&
          typeof data === 'object' &&
          typeof (data as { supported?: unknown }).supported === 'boolean'
        ) {
          setProcesses(data as ProcessList)
        }
      } catch {
        /* silent */
      }
    }
    fetchProcesses()
    const interval = setInterval(fetchProcesses, 15000)
    return () => clearInterval(interval)
  }, [])

  const connectWS = useCallback(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/ws/telemetry`
    )

    ws.onopen = () => setConnected(true)

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        if (!data?.telemetry || !data?.risk) return
        setLatest(data as TelemetryUpdate)
        addPoint(setTempHistory, data.telemetry.temperature_c)
        addPoint(setUtilHistory, data.telemetry.gpu_utilization_pct)
        addPoint(setPowerHistory, data.telemetry.power_draw_w)
        addPoint(setMemoryHistory, data.telemetry.memory_used_pct)
        addPoint(setRiskHistory, data.risk.composite_score)
      } catch {
        /* parse error */
      }
    }

    ws.onclose = () => {
      setConnected(false)
      if (shouldReconnectRef.current) {
        reconnectTimer.current = setTimeout(connectWS, 3000)
      }
    }

    ws.onerror = () => ws.close()
    wsRef.current = ws
  }, [addPoint])

  useEffect(() => {
    shouldReconnectRef.current = true
    connectWS()
    return () => {
      shouldReconnectRef.current = false
      clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connectWS])

  const value: TelemetryContextValue = {
    connected,
    latest,
    status,
    risk,
    processes,
    history: {
      temperature: tempHistory,
      utilization: utilHistory,
      power: powerHistory,
      memory: memoryHistory,
      risk: riskHistory,
    },
    statusLoading,
    statusError,
    refreshHistory,
  }

  return (
    <TelemetryContext.Provider value={value}>
      {children}
    </TelemetryContext.Provider>
  )
}

export function useTelemetry() {
  const ctx = useContext(TelemetryContext)
  if (!ctx) throw new Error('useTelemetry must be used within TelemetryProvider')
  return ctx
}
