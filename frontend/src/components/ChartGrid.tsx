import { useMemo, type ReactNode } from 'react'
import {
  TelemetryChart,
  type ChartEventFlash,
} from '@/components/telemetry-chart'
import type { SparklinePoint } from '@/types/sparkline'
import type { RiskSeverityBand } from '@/types/severity'

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
  tempSeverity: RiskSeverityBand
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

  const tempYDomain = useMemo((): [number, number] => {
    if (chartHistory.temperature.length > 0 && throttleC != null) {
      const values = chartHistory.temperature.map((p) => p.value)
      const minVal = Math.max(
        Math.min(Math.min(...values), throttleC - 20),
        0
      )
      const maxVal = Math.max(Math.max(...values), throttleC + 10, 100)
      return [minVal, maxVal]
    }
    return [0, 100]
  }, [chartHistory.temperature, throttleC])

  const powerYDomain = useMemo((): [number, number] => {
    if (chartHistory.power.length > 0 && tdpW != null) {
      const maxVal = Math.max(
        Math.max(...chartHistory.power.map((p) => p.value)),
        tdpW * 1.1,
        150
      )
      return [0, maxVal]
    }
    return [0, 150]
  }, [chartHistory.power, tdpW])

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
          yDomain={tempYDomain}
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
          yDomain={powerYDomain}
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
