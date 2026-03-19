import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { DeviceDashboard } from '@/pages/DeviceDashboard'
import { DeviceDetail } from '@/pages/DeviceDetail'
import { RiskDrilldown } from '@/pages/RiskDrilldown'

export default function App() {
  return (
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
  )
}
