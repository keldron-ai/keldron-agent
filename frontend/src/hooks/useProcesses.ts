import { useState, useEffect } from 'react'

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

export function useProcesses() {
  const [processes, setProcesses] = useState<ProcessList | null>(null)

  useEffect(() => {
    const fetchProcesses = async () => {
      try {
        const res = await fetch('/api/v1/processes')
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        setProcesses(await res.json())
      } catch {
        /* silent fail — processes are optional */
      }
    }
    fetchProcesses()
    const interval = setInterval(fetchProcesses, 15000)
    return () => clearInterval(interval)
  }, [])

  return processes
}
