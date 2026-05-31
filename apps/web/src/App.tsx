import type { ReactNode } from 'react'
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom'
import AppLayout from './shared/layout/AppLayout'
import AuthLogin from './modules/auth/Login'
import StyleGuide from './modules/styleguide/StyleGuide'
import OwnerDashboard from './modules/owner/Dashboard'
import TemplateDesigner from './modules/template/Designer'
import TemplateList from './modules/template/TemplateList'
import LabelerPlaza from './modules/labeler/Plaza'
import ReviewerQueue from './modules/reviewer/Queue'
import { getToken, hasAnyRole } from './shared/api/client'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Navigate to="/auth/login" replace />} />
        <Route path="/auth/login" element={<AuthLogin />} />
        <Route path="/style-guide" element={<StyleGuide />} />
        <Route element={<RequireAuth />}>
          <Route element={<AppLayout />}>
            <Route path="/owner" element={<Navigate to="/owner/ai" replace />} />
            <Route path="/owner/tasks/:taskId/templates" element={<RequireRole roles={['owner', 'admin']}><TemplateList /></RequireRole>} />
            <Route path="/owner/tasks/:taskId/templates/:templateId" element={<RequireRole roles={['owner', 'admin']}><TemplateDesigner /></RequireRole>} />
            <Route path="/owner/:section" element={<RequireRole roles={['owner', 'admin']}><OwnerDashboard /></RequireRole>} />
            <Route path="/owner/:section/:sub" element={<RequireRole roles={['owner', 'admin']}><OwnerDashboard /></RequireRole>} />
            <Route path="/labeler" element={<RequireRole roles={['labeler']}><LabelerPlaza /></RequireRole>} />
            <Route path="/reviewer" element={<RequireRole roles={['reviewer', 'admin']}><ReviewerQueue /></RequireRole>} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/auth/login" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

function RequireAuth() {
  return getToken() ? <Outlet /> : <Navigate to="/auth/login" replace />
}

function RequireRole({ roles, children }: { roles: string[], children: ReactNode }) {
  return hasAnyRole(roles) ? children : <Navigate to="/auth/login" replace />
}
