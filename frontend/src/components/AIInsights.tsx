import { Sparkles } from 'lucide-react'

export interface AIInsightsProps {
  temperatureC: number
  memoryPct: number
  modelLabel: string
  /** Thermal throttle threshold in °C. Defaults to 105. */
  throttleThreshold?: number
}

export function AIInsights({
  temperatureC,
  memoryPct,
  modelLabel,
  throttleThreshold = 105,
}: AIInsightsProps) {
  const tempInsight =
    temperatureC < 50
      ? `running cool at ${Math.round(temperatureC)}°C with ${Math.round(throttleThreshold - temperatureC)}°C of headroom`
      : temperatureC < throttleThreshold * 0.67
        ? `moderately warm at ${Math.round(temperatureC)}°C — within normal range`
        : `running hot at ${Math.round(temperatureC)}°C — approaching thermal limits`

  const memInsight =
    memoryPct > 90
      ? `Memory pressure is elevated at ${Math.round(memoryPct)}% — consider closing unused applications.`
      : memoryPct > 70
        ? `Memory usage is moderate at ${Math.round(memoryPct)}%.`
        : `Memory usage is healthy at ${Math.round(memoryPct)}%.`

  return (
    <div
      className="rounded-xl border p-2 overflow-hidden"
      style={{
        backgroundColor: '#0F172A',
        borderColor: 'rgba(148, 163, 184, 0.1)',
      }}
    >
      <div className="flex items-center gap-2 mb-1.5">
        <Sparkles className="w-3 h-3 shrink-0 text-[#00C9B0]" aria-hidden />
        <h3
          className="text-[10px] font-bold text-[#00C9B0] uppercase tracking-widest"
          style={{ letterSpacing: '0.08em' }}
        >
          AI Insights
        </h3>
      </div>
      <p className="text-[11px] text-[#CBD5E1] leading-relaxed">
        Your {modelLabel} is {tempInsight}. {memInsight}
      </p>
      <div className="flex items-center gap-1.5 mt-2">
        <div className="w-1.5 h-1.5 rounded-full bg-[#00C9B0]" aria-hidden />
        <span className="text-[9px] text-[#00C9B0]/80 uppercase tracking-widest font-bold">
          Claude Analysis
        </span>
      </div>
    </div>
  )
}
