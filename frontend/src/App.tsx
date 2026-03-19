import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { TelemetryProvider } from '@/context/TelemetryContext'
import { Layout } from '@/components/Layout'
import { DeviceDashboard } from '@/pages/DeviceDashboard'
import { DeviceDetail } from '@/pages/DeviceDetail'
import { RiskDrilldown } from '@/pages/RiskDrilldown'

export default function App() {
  return (
    <TelemetryProvider>
      <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<DeviceDashboard />} />
          <Route path="/device/:id" element={<DeviceDetail />} />
          <Route path="/risk" element={<RiskDrilldown />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
    </TelemetryProvider>
  )
}
