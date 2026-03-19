import { useState, useEffect, useRef, useCallback } from 'react'

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

const SPARKLINE_WINDOW_MS = 30 * 60 * 1000

export function useTelemetryStream() {
  const [connected, setConnected] = useState(false)
  const [latest, setLatest] = useState<TelemetryUpdate | null>(null)

  const [tempHistory, setTempHistory] = useState<SparklinePoint[]>([])
  const [utilHistory, setUtilHistory] = useState<SparklinePoint[]>([])
  const [powerHistory, setPowerHistory] = useState<SparklinePoint[]>([])
  const [memoryHistory, setMemoryHistory] = useState<SparklinePoint[]>([])
  const [riskHistory, setRiskHistory] = useState<SparklinePoint[]>([])

  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>()
  const intentionalClose = useRef(false)

  const addPoint = useCallback(
    (
      setter: React.Dispatch<React.SetStateAction<SparklinePoint[]>>,
      value: number
    ) => {
      const now = Date.now()
      setter((prev) => {
        const cutoff = now - SPARKLINE_WINDOW_MS
        const filtered = prev.filter((p) => p.timestamp > cutoff)
        return [...filtered, { timestamp: now, value }]
      })
    },
    []
  )

  const connect = useCallback(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/ws/telemetry`
    )

    ws.onopen = () => {
      setConnected(true)
      console.log('[WS] Connected')
    }

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        if (!data?.telemetry || !data?.risk) {
          console.log('[WS] Skipping non-telemetry message:', data?.type)
          return
        }
        setLatest(data as TelemetryUpdate)
        addPoint(setTempHistory, data.telemetry.temperature_c)
        addPoint(setUtilHistory, data.telemetry.gpu_utilization_pct)
        addPoint(setPowerHistory, data.telemetry.power_draw_w)
        addPoint(setMemoryHistory, data.telemetry.memory_used_pct)
        addPoint(setRiskHistory, data.risk.composite_score)
      } catch (err) {
        console.warn('[WS] Parse error:', err)
      }
    }

    ws.onclose = () => {
      setConnected(false)
      if (!intentionalClose.current) {
        console.log('[WS] Disconnected, reconnecting in 3s...')
        reconnectTimer.current = setTimeout(connect, 3000)
      }
    }

    ws.onerror = () => {
      ws.close()
    }

    wsRef.current = ws
  }, [addPoint])

  useEffect(() => {
    connect()
    return () => {
      intentionalClose.current = true
      clearTimeout(reconnectTimer.current)
      wsRef.current?.close()
    }
  }, [connect])

  return {
    connected,
    latest,
    history: {
      temperature: tempHistory,
      utilization: utilHistory,
      power: powerHistory,
      memory: memoryHistory,
      risk: riskHistory,
    },
  }
}
