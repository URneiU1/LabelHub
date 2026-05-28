import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Toast } from '@douyinfe/semi-ui'
import { login } from '../../shared/api/client'

const demoAccounts = [
  { username: 'owner1', label: '任务负责人', to: '/owner' },
  { username: 'labeler1', label: '标注员', to: '/labeler' },
  { username: 'reviewer1', label: '审核员', to: '/reviewer' },
]

export default function Login() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState('')

  async function handleLogin(username: string, to: string) {
    setLoading(username)
    try {
      await login(username, 'pass')
      navigate(to)
    } catch (error) {
      Toast.error(error instanceof Error ? error.message : '登录失败')
    } finally {
      setLoading('')
    }
  }

  return (
    <div style={loginPageStyle}>
      <div style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border-light)', padding: 'var(--space-2xl)', maxWidth: 420, width: '100%', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-lg)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', marginBottom: 'var(--space-sm)' }}>
          <div style={logoMarkStyle} aria-hidden="true">
            <span style={logoNodeStyle} />
            <span style={logoNodeMutedStyle} />
            <span style={logoNodeMutedStyle} />
            <span style={logoNodeStyle} />
          </div>
          <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-hero)', margin: 0, letterSpacing: 0 }}>LabelHub</h1>
        </div>
        <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)', marginTop: 'var(--space-sm)', fontSize: 'var(--text-base)' }}>
          欢迎使用数据标注平台
        </p>
        <div style={{ borderTop: '1px solid var(--color-border-light)', marginTop: 'var(--space-xl)', paddingTop: 'var(--space-xl)' }}>
          <div style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)', marginBottom: 'var(--space-md)', fontWeight: 600, textTransform: 'uppercase' }}>选择角色进行演示</div>
          <div style={{ display: 'grid', gap: 'var(--space-md)' }}>
            {demoAccounts.map((account) => (
              <Button
                key={account.username}
                theme="solid"
                size="large"
                loading={loading === account.username}
                onClick={() => void handleLogin(account.username, account.to)}
                style={{ background: 'var(--color-accent)', height: 50, borderRadius: 'var(--radius-md)' }}
              >
                {account.label} · {account.username}
              </Button>
            ))}
          </div>
        </div>
        <div style={{ marginTop: 'var(--space-xl)', textAlign: 'center', fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>
          LabelHub · AI Powered Data Production
        </div>
      </div>
    </div>
  )
}

const loginPageStyle: React.CSSProperties = {
  display: 'flex',
  justifyContent: 'center',
  alignItems: 'center',
  minHeight: '100vh',
  backgroundColor: 'var(--color-bg)',
  backgroundImage: 'linear-gradient(var(--color-grid-line) 1px, transparent 1px), linear-gradient(90deg, var(--color-grid-line) 1px, transparent 1px)',
  backgroundSize: '28px 28px',
  padding: 'var(--space-xl)',
  boxSizing: 'border-box',
}

const logoMarkStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr 1fr',
  gap: 3,
  width: 40,
  height: 40,
  padding: 8,
  border: '1px solid var(--color-border-light)',
  borderRadius: 'var(--radius-md)',
  background: 'var(--color-surface)',
  boxSizing: 'border-box',
}

const logoNodeStyle: React.CSSProperties = {
  background: 'var(--color-accent)',
  borderRadius: 2,
}

const logoNodeMutedStyle: React.CSSProperties = {
  ...logoNodeStyle,
  background: 'var(--color-accent-soft)',
}
