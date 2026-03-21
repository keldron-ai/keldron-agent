import type { ReactNode } from 'react'
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
  /** Fallback height when fillChart is off (non-dashboard). */
  chartHeightClassName?: string
  compactLayout?: boolean
  /** Fill each grid cell vertically (dashboard). */
  fillChart?: boolean
}

function ChartCell({
  children,
}: {
  children: ReactNode
}) {
  return (
    <div className="min-h-[200px] md:min-h-0 h-full min-w-0 flex flex-col">
      {children}
    </div>
  )
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
  chartHeightClassName = 'h-[200px]',
  compactLayout = true,
  fillChart = true,
}: ChartGridProps) {
  const chartProps = {
    compactLayout,
    fillChart,
    chartHeightClassName: fillChart ? undefined : chartHeightClassName,
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 md:grid-rows-[minmax(0,1fr)_minmax(0,1fr)] gap-2 flex-1 min-h-0 h-full w-full">
      <ChartCell>
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
          {...chartProps}
        />
      </ChartCell>
      <ChartCell>
        <TelemetryChart
          title="GPU Utilization"
          data={chartHistory.utilization}
          unit="%"
          color="#3B82F6"
          yDomain={[0, 100]}
          currentValue={util}
          eventFlash={utilChartFlash}
          onEventFlashEnd={onUtilFlashEnd}
          {...chartProps}
        />
      </ChartCell>
      <ChartCell>
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
          {...chartProps}
        />
      </ChartCell>
      <ChartCell>
        <TelemetryChart
          title="Unified Memory"
          data={chartHistory.memory}
          unit="%"
          color="#00E5CC"
          yDomain={[0, 100]}
          currentValue={memPct}
          {...chartProps}
        />
      </ChartCell>
    </div>
  )
}
