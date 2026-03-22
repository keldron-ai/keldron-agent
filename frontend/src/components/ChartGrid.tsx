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
    if (chartHistory.power.length === 0) {
      if (tdpW != null) {
        return [Math.max(0, tdpW - 5), tdpW + 5]
      }
      return [0, 1]
    }
    const values = chartHistory.power.map((p) => p.value)
    const dataMin = Math.min(...values)
    const dataMax = Math.max(...values)
    let minVal = Math.max(0, dataMin - 2)
    let maxVal = dataMax + 5
    if (tdpW != null) {
      maxVal = Math.max(maxVal, tdpW + 5)
      minVal = Math.min(minVal, Math.max(0, tdpW - 5))
    }
    if (minVal >= maxVal) {
      const mid = (dataMin + dataMax) / 2
      minVal = Math.max(0, mid - 1)
      maxVal = mid + 1
    }
    return [minVal, maxVal]
  }, [chartHistory.power, tdpW])

  const memoryYDomain = useMemo((): [number, number] => {
    if (chartHistory.memory.length === 0) return [0, 100]
    const values = chartHistory.memory.map((p) => p.value)
    const dataMin = Math.min(...values)
    const dataMax = Math.max(...values)
    let minVal = Math.max(0, dataMin - 5)
    let maxVal = Math.min(100, dataMax + 2)
    if (minVal >= maxVal) {
      const mid = (dataMin + dataMax) / 2
      minVal = Math.max(0, Math.min(100, mid - 1))
      maxVal = Math.max(0, Math.min(100, mid + 1))
      if (minVal >= maxVal) {
        maxVal = Math.min(minVal + 1, 100)
        if (minVal >= maxVal) minVal = Math.max(0, maxVal - 1)
      }
    }
    return [minVal, maxVal]
  }, [chartHistory.memory])

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
          yDomain={memoryYDomain}
          currentValue={memPct}
          {...chartProps}
        />
      </ChartCell>
    </div>
  )
}
