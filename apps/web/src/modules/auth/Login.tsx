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
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: 'var(--color-bg)' }}>
      <div style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', padding: 'var(--space-2xl)', maxWidth: 420, width: '100%' }}>
        <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)', margin: 0 }}>LabelHub</h1>
        <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)', marginTop: 'var(--space-sm)' }}>
          数据标注平台 · Sprint 1 主线演示
        </p>
        <div style={{ display: 'grid', gap: 'var(--space-sm)', marginTop: 'var(--space-xl)' }}>
          {demoAccounts.map((account) => (
            <Button
              key={account.username}
              theme="solid"
              loading={loading === account.username}
              onClick={() => void handleLogin(account.username, account.to)}
              style={{ background: 'var(--color-accent)', borderColor: 'var(--color-accent)' }}
            >
              {account.label} · {account.username}/pass
            </Button>
          ))}
        </div>
      </div>
    </div>
  )
}

