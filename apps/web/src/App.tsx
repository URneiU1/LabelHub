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
import AIReviewQueue from './modules/reviewer/AIReviewQueue'
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
            <Route path="/owner/export" element={<Navigate to="/owner/export/config" replace />} />
            <Route path="/owner/tasks/:taskId/templates" element={<RequireRole roles={['owner', 'admin']}><TemplateList /></RequireRole>} />
            {/* :templateId 捕获 'new'(空白/复制新建态)与数字版本 id;不要再加字面 /new 路由,否则 templateId 变 undefined 使 Designer 误判 isNew=false。 */}
            <Route path="/owner/tasks/:taskId/templates/:templateId" element={<RequireRole roles={['owner', 'admin']}><TemplateDesigner /></RequireRole>} />
            <Route path="/owner/:section" element={<RequireRole roles={['owner', 'admin']}><OwnerDashboard /></RequireRole>} />
            <Route path="/owner/:section/:sub" element={<RequireRole roles={['owner', 'admin']}><OwnerDashboard /></RequireRole>} />
            {/* 不同 key 让 labeler 三个侧栏入口切换时重挂载,使初始视图(任务广场/标注工作台/我的贡献)按路由落位。 */}
            <Route path="/labeler" element={<RequireRole roles={['labeler']}><LabelerPlaza key="labeler-plaza" initialView="plaza" initialPlazaTab="tasks" /></RequireRole>} />
            <Route path="/labeler/workbench" element={<RequireRole roles={['labeler']}><LabelerPlaza key="labeler-workbench" initialView="answer" /></RequireRole>} />
            <Route path="/labeler/mine" element={<RequireRole roles={['labeler']}><LabelerPlaza key="labeler-mine" initialView="plaza" initialPlazaTab="mydata" /></RequireRole>} />
            {/* 不同 key 让 /reviewer 与 /reviewer/results 切换时重新挂载,使队列初始视图按路由落位。 */}
            <Route path="/reviewer" element={<RequireRole roles={['reviewer', 'admin']}><ReviewerQueue key="reviewer-workbench" /></RequireRole>} />
            <Route path="/reviewer/results" element={<RequireRole roles={['reviewer', 'admin']}><ReviewerQueue key="reviewer-results" /></RequireRole>} />
            <Route path="/reviewer/ai-queue" element={<RequireRole roles={['reviewer', 'admin']}><AIReviewQueue /></RequireRole>} />
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
