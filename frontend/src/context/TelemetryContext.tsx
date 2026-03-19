import React, {
  createContext,
  useContext,
  useState,
  useEffect,
  useRef,
  useCallback,
} from 'react'

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
    severity: 'normal' | 'warning' | 'critical'
    trend: 'stable' | 'rising' | 'falling'
  }
  health?: {
    tdr_celsius: number | null
    tdr_rating: string | null
    stability_celsius: number | null
    perf_per_watt: number | null
  }
}

interface SparklinePoint {
  timestamp: number
  value: number
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
    severity: 'normal' | 'warning' | 'critical'
    trend: 'stable' | 'rising' | 'falling'
    trend_delta: number
  }
  sub_scores: {
    thermal: SubScore
    power: SubScore
    volatility: SubScore
    memory: SubScore
  }
  thresholds: { warning: number; critical: number }
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
}

const TelemetryContext = createContext<TelemetryContextValue | null>(null)

const WINDOW_MS = 30 * 60 * 1000

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

  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>()

  const addPoint = useCallback(
    (
      setter: React.Dispatch<React.SetStateAction<SparklinePoint[]>>,
      value: number
    ) => {
      const now = Date.now()
      setter((prev) => {
        const cutoff = now - WINDOW_MS
        const filtered = prev.filter((p) => p.timestamp > cutoff)
        return [...filtered, { timestamp: now, value }]
      })
    },
    []
  )

  useEffect(() => {
    const loadHistory = async () => {
      try {
        const res = await fetch('/api/v1/history?window=30m')
        if (!res.ok) return
        const data = await res.json()
        if (data.points && data.points.length > 0) {
          const toPoints = (
            arr: Array<Record<string, unknown>>,
            key: string
          ): SparklinePoint[] =>
            arr.map((p) => ({
              timestamp: new Date(p.timestamp as string).getTime(),
              value: (p[key] as number) ?? 0,
            }))

          setTempHistory(toPoints(data.points, 'temperature_c'))
          setUtilHistory(toPoints(data.points, 'gpu_utilization_pct'))
          setPowerHistory(toPoints(data.points, 'power_draw_w'))
          setMemoryHistory(toPoints(data.points, 'memory_used_pct'))
          setRiskHistory(toPoints(data.points, 'composite_score'))
        }
      } catch {
        /* History endpoint may not exist on older agents — silent fail */
      }
    }
    loadHistory()
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
        if (!res.ok) return
        const data = await res.json()
        if (data && typeof data.supported === 'boolean') {
          setProcesses(data)
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
      reconnectTimer.current = setTimeout(connectWS, 3000)
    }

    ws.onerror = () => ws.close()
    wsRef.current = ws
  }, [addPoint])

  useEffect(() => {
    connectWS()
    return () => {
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
