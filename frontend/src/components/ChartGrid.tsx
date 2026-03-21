import {
  TelemetryChart,
  type ChartEventFlash,
} from '@/components/telemetry-chart'
import type { SparklinePoint } from '@/types/sparkline'

export interface ChartGridProps {
  chartHistory: {
    temperature: SparklinePoint[]
    utilization: SparklinePoint[]
    power: SparklinePoint[]
    memory: SparklinePoint[]
  }
  temp: number
  util: number
  power: number
  memPct: number
  throttleC: number | undefined
  tdpW: number | undefined
  tempSeverity: 'normal' | 'warning' | 'critical'
  tempChartFlash: ChartEventFlash | null
  utilChartFlash: ChartEventFlash | null
  onTempFlashEnd: () => void
  onUtilFlashEnd: () => void
  /** Default: taller charts on small screens, compact on md+ */
  chartHeightClassName?: string
  compactLayout?: boolean
}

export function ChartGrid({
  chartHistory,
  temp,
  util,
  power,
  memPct,
  throttleC,
  tdpW,
  tempSeverity,
  tempChartFlash,
  utilChartFlash,
  onTempFlashEnd,
  onUtilFlashEnd,
  chartHeightClassName = 'h-[200px] md:h-[120px]',
  compactLayout = true,
}: ChartGridProps) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-2 flex-1 min-h-0">
      <TelemetryChart
        title="SoC Temperature"
        data={chartHistory.temperature}
        unit="°C"
        color="#00C9B0"
        thresholdValue={throttleC}
        thresholdLabel="Throttle"
        thresholdStrokeColor="#EF4444"
        yDomain={
          chartHistory.temperature.length > 0 && throttleC != null
            ? [
                Math.min(
                  Math.min(...chartHistory.temperature.map((p) => p.value)),
                  throttleC - 20
                ),
                Math.max(
                  Math.max(...chartHistory.temperature.map((p) => p.value)),
                  throttleC + 10,
                  100
                ),
              ]
            : [0, 100]
        }
        currentValue={temp}
        currentValueSeverity={tempSeverity}
        eventFlash={tempChartFlash}
        onEventFlashEnd={onTempFlashEnd}
        chartHeightClassName={chartHeightClassName}
        compactLayout={compactLayout}
      />
      <TelemetryChart
        title="GPU Utilization"
        data={chartHistory.utilization}
        unit="%"
        color="#3B82F6"
        yDomain={[0, 100]}
        currentValue={util}
        eventFlash={utilChartFlash}
        onEventFlashEnd={onUtilFlashEnd}
        chartHeightClassName={chartHeightClassName}
        compactLayout={compactLayout}
      />
      <TelemetryChart
        title="System Power"
        data={chartHistory.power}
        unit="W"
        color="#F59E0B"
        thresholdValue={tdpW}
        thresholdLabel={tdpW != null ? `TDP ${tdpW}W` : undefined}
        thresholdStrokeColor="#F59E0B"
        yDomain={
          chartHistory.power.length > 0 && tdpW != null
            ? [
                0,
                Math.max(
                  Math.max(...chartHistory.power.map((p) => p.value)),
                  tdpW * 1.1,
                  150
                ),
              ]
            : [0, 150]
        }
        currentValue={power}
        chartHeightClassName={chartHeightClassName}
        compactLayout={compactLayout}
      />
      <TelemetryChart
        title="Unified Memory"
        data={chartHistory.memory}
        unit="%"
        color="#00E5CC"
        yDomain={[0, 100]}
        currentValue={memPct}
        chartHeightClassName={chartHeightClassName}
        compactLayout={compactLayout}
      />
    </div>
  )
}
