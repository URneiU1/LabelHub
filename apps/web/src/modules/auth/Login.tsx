export default function Login() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: 'var(--color-bg)' }}>
      <div style={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', padding: 'var(--space-2xl)', maxWidth: 400, width: '100%' }}>
        <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)', margin: 0 }}>LabelHub</h1>
        <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)', marginTop: 'var(--space-sm)' }}>
          数据标注平台 · Sprint 0 骨架
        </p>
        <div style={{ marginTop: 'var(--space-xl)', padding: 'var(--space-md)', background: 'var(--color-bg)', border: '1px solid var(--color-border-light)' }}>
          <p style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', margin: 0 }}>Sprint 1 实现 JWT 登录</p>
          <p style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)', margin: 'var(--space-xs) 0 0 0' }}>
            当前阶段:角色切换按钮直通
          </p>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-lg)', flexDirection: 'column' }}>
          <a href="/owner" style={linkStyle}>→ 任务负责人后台</a>
          <a href="/labeler" style={linkStyle}>→ 标注员工作台</a>
          <a href="/reviewer" style={linkStyle}>→ 人工审核中心</a>
          <a href="/style-guide" style={linkStyle}>→ Editorial 风格指南</a>
        </div>
      </div>
    </div>
  )
}

const linkStyle: React.CSSProperties = {
  display: 'block',
  padding: 'var(--space-md)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-text)',
  textDecoration: 'none',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-base)',
  background: 'var(--color-bg)',
}
