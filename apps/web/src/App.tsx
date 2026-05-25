import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom'
import AppLayout from './shared/layout/AppLayout'
import AuthLogin from './modules/auth/Login'
import StyleGuide from './modules/styleguide/StyleGuide'
import OwnerDashboard from './modules/owner/Dashboard'
import TemplateDesigner from './modules/template/Designer'
import TemplateList from './modules/template/TemplateList'
import LabelerPlaza from './modules/labeler/Plaza'
import ReviewerQueue from './modules/reviewer/Queue'
import { getToken } from './shared/api/client'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Navigate to="/auth/login" replace />} />
        <Route path="/auth/login" element={<AuthLogin />} />
        <Route path="/style-guide" element={<StyleGuide />} />
        <Route element={<RequireAuth />}>
          <Route element={<AppLayout />}>
            <Route path="/owner" element={<OwnerDashboard />} />
            <Route path="/owner/tasks/:taskId/templates" element={<TemplateList />} />
            <Route path="/owner/tasks/:taskId/templates/:templateId" element={<TemplateDesigner />} />
            <Route path="/labeler" element={<LabelerPlaza />} />
            <Route path="/reviewer" element={<ReviewerQueue />} />
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
