import { useEffect, useState } from 'react'
import { Activity, ChevronDown, HardDrive, Thermometer, Zap } from 'lucide-react'
import { SubScoreBars } from '@/components/SubScoreBars'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import type { SubScore, SubScores } from '@/types/risk'
import { subScoreColor } from '@/types/severity'

function fmt1(n: unknown): string {
  if (typeof n !== 'number' || !Number.isFinite(n)) return '—'
  return n.toFixed(1)
}

function fmtBytesGB(n: unknown): string {
  if (typeof n !== 'number' || !Number.isFinite(n)) return '—'
  return `${(n / 1073741824).toFixed(1)} GB`
}

function getNumeric(
  d: Record<string, unknown>,
  key: string
): number | undefined {
  const v = d[key]
  if (typeof v === 'number' && Number.isFinite(v)) return v
  return undefined
}

function fmtWithUnit(formatter: (n: unknown) => string, value: unknown, unit: string): string {
  const v = formatter(value)
  return v === '—' ? v : `${v}${unit}`
}

function DetailRow({
  label,
  value,
}: {
  label: string
  value: string
}) {
  return (
    <div className="flex justify-between gap-2 text-[10px] pl-2 border-l border-white/[0.06]">
      <span className="text-[#94A3B8]">{label}</span>
      <span className="text-[#E8ECF4] tabular-nums shrink-0">{value}</span>
    </div>
  )
}

function SubScoreHeader({
  icon,
  label,
  data,
}: {
  icon: React.ReactNode
  label: string
  data: SubScore
}) {
  return (
    <p className="text-[11px] text-[#E8ECF4]">
      <span className="inline-flex items-center gap-1 text-[#94A3B8]">
        {icon}
        {label}
      </span>{' '}
      <span style={{ color: subScoreColor(data.score) }}>● {fmt1(data.score)}</span>
      <span className="text-[#94A3B8]">
        {' '}
        × {fmt1(data.weight)} = {fmt1(data.weighted_contribution)} pts
      </span>
    </p>
  )
}

function ThermalBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  const t = getNumeric(d, 'current_temp_c')
  const th = getNumeric(d, 'throttle_threshold_c')
  const hr = getNumeric(d, 'headroom_pct')
  const roc = getNumeric(d, 'roc_penalty')
  return (
    <div className="space-y-1">
      <SubScoreHeader
        icon={<Thermometer className="w-3 h-3" aria-hidden />}
        label="Thermal"
        data={data}
      />
      <DetailRow label="Current Temp" value={fmtWithUnit(fmt1, t, '°C')} />
      <DetailRow label="Throttle Limit" value={fmtWithUnit(fmt1, th, '°C')} />
      <DetailRow label="Headroom" value={hr != null ? fmtWithUnit(fmt1, hr, '%') : '—'} />
      <DetailRow label="RoC Penalty" value={fmt1(roc)} />
    </div>
  )
}

function PowerBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  return (
    <div className="space-y-1">
      <SubScoreHeader
        icon={<Zap className="w-3 h-3" aria-hidden />}
        label="Power"
        data={data}
      />
      <DetailRow label="Current Power" value={fmtWithUnit(fmt1, getNumeric(d, 'current_power_w'), 'W')} />
      <DetailRow label="TDP" value={fmtWithUnit(fmt1, getNumeric(d, 'tdp_w'), 'W')} />
      <DetailRow label="Utilization" value={fmtWithUnit(fmt1, getNumeric(d, 'utilization_pct'), '%')} />
    </div>
  )
}

function VolatilityBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  const cv = getNumeric(d, 'cv')
  return (
    <div className="space-y-1">
      <SubScoreHeader
        icon={<Activity className="w-3 h-3" aria-hidden />}
        label="Volatility"
        data={data}
      />
      <DetailRow label="CV" value={fmt1(cv)} />
      <DetailRow
        label="Window"
        value={fmtWithUnit(fmt1, getNumeric(d, 'window_minutes'), ' min')}
      />
    </div>
  )
}

function MemoryBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  return (
    <div className="space-y-1">
      <SubScoreHeader
        icon={<HardDrive className="w-3 h-3" aria-hidden />}
        label="Memory"
        data={data}
      />
      <DetailRow label="Memory Used" value={fmtBytesGB(getNumeric(d, 'memory_used_bytes'))} />
      <DetailRow label="Memory Total" value={fmtBytesGB(getNumeric(d, 'memory_total_bytes'))} />
      <DetailRow label="Used Pct" value={fmtWithUnit(fmt1, getNumeric(d, 'memory_used_pct'), '%')} />
    </div>
  )
}

export interface SubScoresPanelProps {
  subScores: SubScores | null | undefined
  riskSummaryLine: string
  layerOneNote?: string
}

export function SubScoresPanel({
  subScores,
  riskSummaryLine,
  layerOneNote = 'Layer 1 local scoring',
}: SubScoresPanelProps) {
  const [detailOpen, setDetailOpen] = useState(false)

  useEffect(() => {
    if (!subScores) {
      setDetailOpen(false)
    }
  }, [subScores])

  return (
    <div
      className="rounded-xl border p-2 flex flex-col min-h-0 overflow-hidden"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <Popover open={detailOpen} onOpenChange={setDetailOpen}>
        <div className="flex items-start justify-between gap-2 mb-1.5">
          <h3
            className="text-[10px] font-bold text-[#94A3B8] uppercase tracking-widest shrink"
            style={{ letterSpacing: '0.08em' }}
          >
            Risk Sub-scores
          </h3>
          <PopoverTrigger asChild>
            <button
              type="button"
              disabled={!subScores}
              className={cn(
                'flex items-center gap-0.5 text-[9px] font-bold uppercase tracking-wider shrink-0',
                subScores
                  ? 'text-[#00C9B0] hover:text-[#00E5CC]'
                  : 'text-[#64748B] cursor-not-allowed'
              )}
            >
              {detailOpen ? 'Hide detail' : 'Show detail'}
              <ChevronDown
                className={`w-3 h-3 transition-transform ${detailOpen ? 'rotate-180' : ''}`}
                aria-hidden
              />
            </button>
          </PopoverTrigger>
        </div>

        <div className="capitalize">
          <SubScoreBars subScores={subScores} />
        </div>

        <p className="text-[10px] text-[#94A3B8] mt-1.5 leading-snug">{riskSummaryLine}</p>
        <p className="text-[9px] text-[#64748B] mt-0.5">{layerOneNote}</p>

        {subScores && (
          <PopoverContent
            align="end"
            side="bottom"
            sideOffset={6}
            className="w-[min(32rem,calc(100vw-1rem))] p-0 border border-white/[0.1] bg-[#0F172A] text-[#E8ECF4] shadow-xl z-[100]"
          >
            <div
              className="p-4 space-y-4 max-h-[min(72vh,560px)] overflow-y-auto pr-1"
              role="region"
              aria-label="Risk sub-score breakdown"
            >
              <ThermalBlock data={subScores.thermal} />
              <PowerBlock data={subScores.power} />
              <VolatilityBlock data={subScores.volatility} />
              <MemoryBlock data={subScores.memory} />
            </div>
          </PopoverContent>
        )}
      </Popover>
    </div>
  )
}
