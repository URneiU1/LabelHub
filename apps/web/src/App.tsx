import { BrowserRouter, Routes, Route } from 'react-router-dom'
import AppLayout from './shared/layout/AppLayout'
import AuthLogin from './modules/auth/Login'
import StyleGuide from './modules/styleguide/StyleGuide'
import OwnerDashboard from './modules/owner/Dashboard'
import LabelerPlaza from './modules/labeler/Plaza'
import ReviewerQueue from './modules/reviewer/Queue'

export default function App() {
  return (
    <BrowserRouter>
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
    </BrowserRouter>
  )
}
