export default function OwnerDashboard() {
  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>Owner 任务负责人</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>任务管理 · 模板搭建 · 审核配置 · 数据看板 · 导出</p>
      <div style={{ marginTop: 'var(--space-xl)', padding: 'var(--space-xl)', background: 'var(--color-surface)', border: '1px solid var(--color-border)', textAlign: 'center' }}>
        <p style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h2)', color: 'var(--color-text-muted)' }}>Sprint 1 实现</p>
      </div>
    </div>
  )
}
