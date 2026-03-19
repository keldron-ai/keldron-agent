import { useMemo, useState } from 'react'
import { ChevronDown, ChevronUp, Cpu } from 'lucide-react'

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

type SortKey = 'gpu_memory' | 'gpu_util' | 'runtime' | null

function ariaSortFor(
  column: Exclude<SortKey, null>,
  sortKey: SortKey,
  dir: 'asc' | 'desc'
): 'ascending' | 'descending' | 'none' {
  if (sortKey !== column) return 'none'
  return dir === 'asc' ? 'ascending' : 'descending'
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
  const [sortKey, setSortKey] = useState<SortKey>(null)
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('desc')

  const sortedProcesses = useMemo(() => {
    if (!sortKey) return processes
    const mult = sortDirection === 'asc' ? 1 : -1
    return [...processes].sort((a, b) => {
      const va =
        sortKey === 'gpu_memory'
          ? a.gpu_memory_bytes
          : sortKey === 'gpu_util'
            ? a.gpu_utilization_pct
            : a.runtime_seconds
      const vb =
        sortKey === 'gpu_memory'
          ? b.gpu_memory_bytes
          : sortKey === 'gpu_util'
            ? b.gpu_utilization_pct
            : b.runtime_seconds
      return mult * (va - vb)
    })
  }, [processes, sortKey, sortDirection])

  const handleSortChange = (key: Exclude<SortKey, null>) => {
    if (sortKey === key) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDirection('desc')
    }
  }

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
            <th
              className="text-right font-medium px-5 pb-3"
              aria-sort={ariaSortFor('gpu_memory', sortKey, sortDirection)}
            >
              <button
                type="button"
                onClick={() => handleSortChange('gpu_memory')}
                className="inline-flex items-center justify-end gap-0.5 w-full uppercase text-[11px] text-[#94A3B8] hover:text-[#E8ECF4] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#00C9B0] rounded-sm"
              >
                GPU Memory
                {sortKey === 'gpu_memory' &&
                  (sortDirection === 'asc' ? (
                    <ChevronUp className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
                  ) : (
                    <ChevronDown className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
                  ))}
              </button>
            </th>
            <th
              className="text-right font-medium px-5 pb-3"
              aria-sort={ariaSortFor('gpu_util', sortKey, sortDirection)}
            >
              <button
                type="button"
                onClick={() => handleSortChange('gpu_util')}
                className="inline-flex items-center justify-end gap-0.5 w-full uppercase text-[11px] text-[#94A3B8] hover:text-[#E8ECF4] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#00C9B0] rounded-sm"
              >
                GPU %
                {sortKey === 'gpu_util' &&
                  (sortDirection === 'asc' ? (
                    <ChevronUp className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
                  ) : (
                    <ChevronDown className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
                  ))}
              </button>
            </th>
            <th
              className="text-right font-medium px-5 pb-3"
              aria-sort={ariaSortFor('runtime', sortKey, sortDirection)}
            >
              <button
                type="button"
                onClick={() => handleSortChange('runtime')}
                className="inline-flex items-center justify-end gap-0.5 w-full uppercase text-[11px] text-[#94A3B8] hover:text-[#E8ECF4] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#00C9B0] rounded-sm"
              >
                Runtime
                {sortKey === 'runtime' &&
                  (sortDirection === 'asc' ? (
                    <ChevronUp className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
                  ) : (
                    <ChevronDown className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
                  ))}
              </button>
            </th>
            <th className="text-left font-medium px-5 pb-3">User</th>
          </tr>
        </thead>
        <tbody>
          {sortedProcesses.map((proc, i) => (
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
