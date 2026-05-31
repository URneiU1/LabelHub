import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Toast } from '@douyinfe/semi-ui'
import { login } from '../../shared/api/client'

const demoAccounts = [
  { username: 'owner1', label: '任务负责人', to: '/owner' },
  { username: 'labeler1', label: '标注员', to: '/labeler' },
  { username: 'reviewer1', label: '人工审核员', to: '/reviewer' },
]

const demoPassword = '123456'

// 登录成功后按角色落到对应工作区(密码登录用,demo 一键登录用固定 to)。
function homeForRoles(roles: string[]): string {
  if (roles.includes('owner') || roles.includes('admin')) return '/owner'
  if (roles.includes('reviewer')) return '/reviewer'
  if (roles.includes('labeler')) return '/labeler'
  return '/owner'
}

export default function Login() {
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [quickLoading, setQuickLoading] = useState('')

  async function handlePasswordLogin(event: FormEvent) {
    event.preventDefault()
    if (submitting) return
    if (!username.trim() || !password) {
      Toast.error('请输入用户名和密码')
      return
    }
    setSubmitting(true)
    try {
      const account = await login(username.trim(), password)
      navigate(homeForRoles(account.roles ?? []))
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '登录失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleQuickLogin(user: string, to: string) {
    setQuickLoading(user)
    try {
      await login(user, demoPassword)
      navigate(to)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '登录失败')
    } finally {
      setQuickLoading('')
    }
  }

  return (
    <div style={pageStyle}>
      <div style={cardStyle}>
        <div className="lh-topnav__brand" style={{ fontSize: 20 }}>
          <span className="lh-topnav__brand-dot" />
          <span>LabelHub</span>
        </div>
        <p style={{ color: 'var(--lh-text-2)', marginTop: 'var(--space-sm)', fontSize: 'var(--text-base)' }}>
          数据标注平台 · 登录
        </p>

        <form onSubmit={(event) => void handlePasswordLogin(event)} style={{ marginTop: 'var(--space-xl)', display: 'grid', gap: 'var(--space-md)' }}>
          <label style={labelStyle}>
            <span style={labelTextStyle}>用户名</span>
            <input
              aria-label="用户名"
              autoComplete="username"
              style={inputStyle}
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              placeholder="如 owner1"
            />
          </label>
          <label style={labelStyle}>
            <span style={labelTextStyle}>密码</span>
            <input
              aria-label="密码"
              type="password"
              autoComplete="current-password"
              style={inputStyle}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="请输入密码"
            />
          </label>
          <button type="submit" className="lh-btn lh-btn--primary lh-btn--lg" disabled={submitting} style={fullWidthButtonStyle}>
            {submitting ? '登录中…' : '登录'}
          </button>
        </form>

        <div style={{ borderTop: '1px solid var(--lh-border)', marginTop: 'var(--space-xl)', paddingTop: 'var(--space-lg)' }}>
          <div style={hintStyle}>演示账号一键登录(密码 {demoPassword})</div>
          <div style={{ display: 'grid', gap: 'var(--space-sm)' }}>
            {demoAccounts.map((account) => (
              <button
                key={account.username}
                type="button"
                className="lh-btn"
                disabled={quickLoading === account.username}
                onClick={() => void handleQuickLogin(account.username, account.to)}
                style={fullWidthButtonStyle}
              >
                {account.label} · {account.username}
              </button>
            ))}
          </div>
        </div>

        <div style={{ marginTop: 'var(--space-xl)', textAlign: 'center', fontSize: 'var(--text-sm)', color: 'var(--lh-text-3)' }}>
          LabelHub · AI Powered Data Production
        </div>
      </div>
    </div>
  )
}

const pageStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'center',
  alignItems: 'center',
  minHeight: '100vh',
  background: 'var(--lh-bg)',
  padding: 'var(--space-xl)',
  boxSizing: 'border-box',
}

const cardStyle: React.CSSProperties = {
  background: 'var(--lh-bg-card)',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--radius-lg)',
  boxShadow: 'var(--shadow-md)',
  padding: 'var(--space-xl)',
  maxWidth: 400,
  width: '100%',
  boxSizing: 'border-box',
  fontFamily: 'var(--lh-font-sans)',
}

const labelStyle: React.CSSProperties = {
  display: 'grid',
  gap: 6,
}

const labelTextStyle: React.CSSProperties = {
  fontSize: 'var(--text-sm)',
  color: 'var(--lh-text-2)',
  fontWeight: 600,
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  border: '1px solid var(--lh-border)',
  borderRadius: 'var(--lh-radius)',
  padding: '9px 12px',
  fontSize: 'var(--text-base)',
  fontFamily: 'var(--lh-font-sans)',
  background: '#fff',
  color: 'var(--lh-text-1)',
  boxSizing: 'border-box',
}

const fullWidthButtonStyle: React.CSSProperties = {
  width: '100%',
  justifyContent: 'center',
}

const hintStyle: React.CSSProperties = {
  fontSize: 'var(--text-sm)',
  color: 'var(--lh-text-3)',
  marginBottom: 'var(--space-md)',
}
