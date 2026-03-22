import { useState } from 'react'
import { Activity, ChevronDown, HardDrive, Thermometer, Zap } from 'lucide-react'
import { SubScoreBars } from '@/components/SubScoreBars'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

interface SubScore {
  score: number
  weight: number
  weighted_contribution: number
  details: Record<string, unknown>
}

interface SubScores {
  thermal: SubScore
  power: SubScore
  volatility: SubScore
  memory: SubScore
}

function getSeverityColor(score: number): string {
  if (score < 40) return '#22C55E'
  if (score < 70) return '#F59E0B'
  return '#EF4444'
}

function fmt1(n: unknown): string {
  if (typeof n !== 'number' || !Number.isFinite(n)) return '—'
  return n.toFixed(1)
}

function fmtBytesGB(n: unknown): string {
  if (typeof n !== 'number' || !Number.isFinite(n)) return '—'
  return `${(n / 1073741824).toFixed(1)} GB`
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

function ThermalBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  const t = d.current_temp_c
  const th = d.throttle_threshold_c
  const hr = d.headroom_pct
  const roc = d.roc_penalty
  return (
    <div className="space-y-1">
      <p className="text-[11px] text-[#E8ECF4]">
        <span className="inline-flex items-center gap-1 text-[#94A3B8]">
          <Thermometer className="w-3 h-3" aria-hidden />
          Thermal
        </span>{' '}
        <span style={{ color: getSeverityColor(data.score) }}>● {fmt1(data.score)}</span>
        <span className="text-[#94A3B8]">
          {' '}
          × {fmt1(data.weight)} = {fmt1(data.weighted_contribution)} pts
        </span>
      </p>
      <DetailRow label="Current Temp" value={`${fmt1(t)}°C`} />
      <DetailRow label="Throttle Limit" value={`${fmt1(th)}°C`} />
      <DetailRow label="Headroom" value={hr != null && typeof hr === 'number' ? `${fmt1(hr)}%` : '—'} />
      <DetailRow label="RoC Penalty" value={fmt1(roc)} />
    </div>
  )
}

function PowerBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  return (
    <div className="space-y-1">
      <p className="text-[11px] text-[#E8ECF4]">
        <span className="inline-flex items-center gap-1 text-[#94A3B8]">
          <Zap className="w-3 h-3" aria-hidden />
          Power
        </span>{' '}
        <span style={{ color: getSeverityColor(data.score) }}>● {fmt1(data.score)}</span>
        <span className="text-[#94A3B8]">
          {' '}
          × {fmt1(data.weight)} = {fmt1(data.weighted_contribution)} pts
        </span>
      </p>
      <DetailRow label="Current Power" value={`${fmt1(d.current_power_w)}W`} />
      <DetailRow label="TDP" value={`${fmt1(d.tdp_w)}W`} />
      <DetailRow label="Utilization" value={`${fmt1(d.utilization_pct)}%`} />
    </div>
  )
}

function VolatilityBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  const cv = d.cv
  const cvStr =
    cv == null || (typeof cv === 'number' && !Number.isFinite(cv))
      ? '—'
      : fmt1(cv)
  return (
    <div className="space-y-1">
      <p className="text-[11px] text-[#E8ECF4]">
        <span className="inline-flex items-center gap-1 text-[#94A3B8]">
          <Activity className="w-3 h-3" aria-hidden />
          Volatility
        </span>{' '}
        <span style={{ color: getSeverityColor(data.score) }}>● {fmt1(data.score)}</span>
        <span className="text-[#94A3B8]">
          {' '}
          × {fmt1(data.weight)} = {fmt1(data.weighted_contribution)} pts
        </span>
      </p>
      <DetailRow label="CV" value={cvStr} />
      <DetailRow
        label="Window"
        value={
          d.window_minutes != null && typeof d.window_minutes === 'number'
            ? `${fmt1(d.window_minutes)} min`
            : '—'
        }
      />
    </div>
  )
}

function MemoryBlock({ data }: { data: SubScore }) {
  const d = data.details ?? {}
  return (
    <div className="space-y-1">
      <p className="text-[11px] text-[#E8ECF4]">
        <span className="inline-flex items-center gap-1 text-[#94A3B8]">
          <HardDrive className="w-3 h-3" aria-hidden />
          Memory
        </span>{' '}
        <span style={{ color: getSeverityColor(data.score) }}>● {fmt1(data.score)}</span>
        <span className="text-[#94A3B8]">
          {' '}
          × {fmt1(data.weight)} = {fmt1(data.weighted_contribution)} pts
        </span>
      </p>
      <DetailRow label="Memory Used" value={fmtBytesGB(d.memory_used_bytes)} />
      <DetailRow label="Memory Total" value={fmtBytesGB(d.memory_total_bytes)} />
      <DetailRow label="Used Pct" value={`${fmt1(d.memory_used_pct)}%`} />
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
            className="w-[min(28rem,calc(100vw-1rem))] p-0 border border-white/[0.1] bg-[#0F172A] text-[#E8ECF4] shadow-xl z-[100]"
          >
            <div
              className="p-4 space-y-4 max-h-[min(40vh,320px)] overflow-y-auto pr-1"
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
