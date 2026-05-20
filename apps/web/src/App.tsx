import { Suspense, lazy } from 'react'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Spin } from '@douyinfe/semi-ui'
import AppLayout from './shared/layout/AppLayout'

const AuthLogin = lazy(() => import('./modules/auth/Login'))
const StyleGuide = lazy(() => import('./modules/styleguide/StyleGuide'))
const OwnerDashboard = lazy(() => import('./modules/owner/Dashboard'))
const LabelerPlaza = lazy(() => import('./modules/labeler/Plaza'))
const ReviewerQueue = lazy(() => import('./modules/reviewer/Queue'))

function Fallback() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', background: 'var(--color-bg)' }}>
      <Spin size="large" />
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Suspense fallback={<Fallback />}>
        <Routes>
          <Route path="/auth/login" element={<AuthLogin />} />
          <Route path="/style-guide" element={<StyleGuide />} />
          <Route element={<AppLayout />}>
            <Route path="/owner" element={<OwnerDashboard />} />
            <Route path="/labeler" element={<LabelerPlaza />} />
            <Route path="/reviewer" element={<ReviewerQueue />} />
            <Route path="*" element={<AuthLogin />} />
          </Route>
        </Routes>
      </Suspense>
    </BrowserRouter>
  )
}
