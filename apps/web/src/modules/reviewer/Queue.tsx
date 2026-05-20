export default function ReviewerQueue() {
  return (
    <div>
      <h1 style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h1)' }}>人工审核中心</h1>
      <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-secondary)' }}>审核队列 · 通过 / 打回 / 修订 · 批量操作 · 审计时间线</p>
      <div style={{ marginTop: 'var(--space-xl)', padding: 'var(--space-xl)', background: 'var(--color-surface)', border: '1px solid var(--color-border)', textAlign: 'center' }}>
        <p style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h2)', color: 'var(--color-text-muted)' }}>Sprint 1 实现</p>
      </div>
    </div>
  )
}
