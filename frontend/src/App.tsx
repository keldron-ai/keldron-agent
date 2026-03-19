import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { DeviceDashboard } from '@/pages/DeviceDashboard'
import { RiskDrilldown } from '@/pages/RiskDrilldown'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<DeviceDashboard />} />
          <Route path="/risk" element={<RiskDrilldown />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
