import { Cpu } from 'lucide-react'

interface Process {
  pid: number
  name: string
  gpu_memory_bytes: number
  gpu_utilization_pct: number
  runtime_seconds: number
  user: string
}

interface ProcessTableProps {
  processes: Process[]
  supported: boolean
  note: string | null
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

function formatRuntime(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  const hours = Math.floor(seconds / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  return `${hours}h ${mins}m`
}

export function ProcessTable({
  processes,
  supported,
  note,
}: ProcessTableProps) {
  if (!supported) {
    return (
      <div
        className="rounded-xl border p-5 flex items-center gap-3"
        style={{
          backgroundColor: '#0F172A',
          borderColor: 'rgba(148, 163, 184, 0.1)',
        }}
      >
        <Cpu className="w-8 h-8 text-[#64748B] shrink-0" />
        <p className="text-sm text-[#94A3B8]">
          {note ?? 'Process enumeration not available for this adapter'}
        </p>
      </div>
    )
  }

  if (processes.length === 0) {
    return (
      <div
        className="rounded-xl border p-5"
        style={{
          backgroundColor: '#0F172A',
          borderColor: 'rgba(148, 163, 184, 0.1)',
        }}
      >
        <h3 className="text-sm font-semibold text-[#E8ECF4] mb-2">
          Active Processes
        </h3>
        <p className="text-sm text-[#94A3B8]">No active GPU processes</p>
      </div>
    )
  }

  return (
    <div
      className="rounded-xl border overflow-hidden"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <h3 className="text-sm font-semibold text-[#E8ECF4] px-5 py-4">
        Active Processes
      </h3>
      <table className="w-full">
        <thead>
          <tr className="text-[11px] text-[#94A3B8] uppercase">
            <th className="text-left font-medium px-5 pb-3">Process Name</th>
            <th className="text-left font-medium px-5 pb-3">PID</th>
            <th className="text-right font-medium px-5 pb-3">GPU Memory</th>
            <th className="text-right font-medium px-5 pb-3">GPU %</th>
            <th className="text-right font-medium px-5 pb-3">Runtime</th>
            <th className="text-left font-medium px-5 pb-3">User</th>
          </tr>
        </thead>
        <tbody>
          {processes.map((proc, i) => (
            <tr
              key={`${proc.pid}-${proc.name}`}
              className="border-t border-white/[0.06] hover:bg-white/[0.04] transition-colors"
              style={{
                backgroundColor: i % 2 === 1 ? 'rgba(255,255,255,0.02)' : undefined,
              }}
            >
              <td className="px-5 py-3">
                <span className="text-sm font-medium text-[#E8ECF4]">
                  {proc.name}
                </span>
              </td>
              <td className="px-5 py-3">
                <span className="text-sm font-mono text-[#94A3B8]">
                  {proc.pid}
                </span>
              </td>
              <td className="px-5 py-3 text-right text-sm text-[#E8ECF4]">
                {formatBytes(proc.gpu_memory_bytes)}
              </td>
              <td className="px-5 py-3 text-right">
                <div className="flex items-center justify-end gap-2">
                  <div className="w-12 h-1.5 bg-white/10 rounded-full overflow-hidden">
                    <div
                      className="h-full rounded-full"
                      style={{
                        width: `${Math.min(100, proc.gpu_utilization_pct)}%`,
                        backgroundColor:
                          proc.gpu_utilization_pct > 80 ? '#EF4444' : '#00C9B0',
                      }}
                    />
                  </div>
                  <span
                    className="text-sm font-medium w-10 text-right"
                    style={{
                      color:
                        proc.gpu_utilization_pct > 80 ? '#EF4444' : '#E8ECF4',
                    }}
                  >
                    {proc.gpu_utilization_pct.toFixed(1)}%
                  </span>
                </div>
              </td>
              <td className="px-5 py-3 text-right text-sm text-[#E8ECF4]">
                {formatRuntime(proc.runtime_seconds)}
              </td>
              <td className="px-5 py-3 text-sm text-[#94A3B8]">
                {proc.user || '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
