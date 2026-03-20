import { useEffect, useMemo, useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Link } from 'react-router-dom'
import { useTelemetry } from '@/context/TelemetryContext'

const STALE_MS = 15_000

export function Layout() {
  const { status, connected, latest } = useTelemetry()
  const [, setNowTick] = useState(0)

  useEffect(() => {
    const id = window.setInterval(() => setNowTick((n) => n + 1), 1000)
    return () => window.clearInterval(id)
  }, [])

  const lastDataMs = useMemo(() => {
    const times: number[] = []
    if (latest?.timestamp) {
      const t = new Date(latest.timestamp).getTime()
      if (Number.isFinite(t)) times.push(t)
    }
    if (status?.telemetry?.timestamp) {
      const t = new Date(status.telemetry.timestamp).getTime()
      if (Number.isFinite(t)) times.push(t)
    }
    return times.length ? Math.max(...times) : null
  }, [latest?.timestamp, status?.telemetry?.timestamp])

  const isFresh =
    connected &&
    lastDataMs != null &&
    Date.now() - lastDataMs < STALE_MS

  const hostname = status?.device?.hostname ?? '—'
  const adapter = status?.device?.adapter ?? '—'
  const version = status?.agent?.version ?? '—'

  const statusLabel = !connected
    ? 'Reconnecting…'
    : isFresh
      ? 'Live'
      : 'Stale'

  return (
    <div className="min-h-screen bg-[#0A0C10] flex flex-col">
      <header className="h-14 bg-[#0F172A] border-b border-white/[0.06] flex items-center justify-between px-6 shrink-0">
        <div className="flex items-center gap-4">
          <Link to="/" className="flex items-center gap-2">
            <svg
              width="28"
              height="28"
              viewBox="0 0 28 28"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
            >
              <polygon
                points="7,0 21,0 28,14 21,28 7,28 0,14"
                fill="#00C9B0"
              />
              <text
                x="14"
                y="19"
                textAnchor="middle"
                fill="#0A0C10"
                fontSize="14"
                fontWeight="700"
                fontFamily="system-ui, sans-serif"
              >
                K
              </text>
            </svg>
            <span className="text-lg font-semibold text-[#E8ECF4]">
              Keldron
            </span>
          </Link>
          <div className="h-5 w-px bg-white/[0.06]" />
          <div className="flex items-center gap-2 text-sm text-[#94A3B8]">
            <span className="font-medium text-[#E8ECF4]">{hostname}</span>
            <span className="bg-white/[0.08] rounded-full px-2.5 py-0.5 text-[11px]">
              {adapter}
            </span>
          </div>
        </div>

        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2 text-sm">
            <span className="inline-flex h-3 w-3 shrink-0 items-center justify-center overflow-visible">
              <span
                className={`h-2 w-2 rounded-full ${
                  isFresh ? 'bg-[#00C9B0] animate-live-dot-pulse' : 'bg-[#64748B]'
                }`}
              />
            </span>
            <span className="text-[#94A3B8]">{statusLabel}</span>
          </div>
          <div className="flex items-center gap-2 text-[11px] text-[#64748B]">
            <span>v{version}</span>
            <span>·</span>
            <span>{status?.agent?.cloud_connected ? 'Cloud' : 'Local mode'}</span>
          </div>
          {status?.agent?.cloud_connected && (
            <a
              href="https://keldron.ai"
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-[#00C9B0] hover:text-[#00E5CC] transition-colors"
            >
              Keldron Cloud →
            </a>
          )}
        </div>
      </header>

      <main className="flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}
