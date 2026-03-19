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
    const controller = new AbortController()
    const fetchProcesses = async () => {
      try {
        const res = await fetch('/api/v1/processes', { signal: controller.signal })
        let data: ProcessList | null = null
        try {
          data = await res.json()
        } catch {
          /* body may be empty or invalid */
        }
        if (res.ok && data) {
          setProcesses(data)
        } else if (data && typeof data.supported === 'boolean') {
          setProcesses(data)
        }
      } catch {
        /* silent fail — processes are optional */
      }
    }
    fetchProcesses()
    const interval = setInterval(fetchProcesses, 15000)
    return () => {
      controller.abort()
      clearInterval(interval)
    }
  }, [])

  return processes
}
