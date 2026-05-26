import { Button, Card, Tag, Typography } from '@douyinfe/semi-ui'

const { Title, Paragraph } = Typography

export default function StyleGuide() {
  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: 'var(--space-2xl) var(--space-xl)', background: 'var(--color-bg)', minHeight: '100vh' }}>
      <div style={{ background: 'var(--color-surface)', padding: 'var(--space-2xl)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={1} style={{ fontFamily: 'var(--font-heading)', fontWeight: 700, letterSpacing: 0 }}>Schematic Enterprise Style Guide</Title>
        <Paragraph style={{ color: 'var(--color-text-secondary)', fontFamily: 'var(--font-body)', fontSize: 'var(--text-h2)' }}>
          LabelHub Schematic 风格令牌 · 控制台分区 · 画布网格 · 节点卡片 · 状态轨道
        </Paragraph>
      </div>

      {/* Color Palette */}
      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>色彩 Palette</Title>
        <div style={{ display: 'flex', gap: 'var(--space-lg)', flexWrap: 'wrap', marginTop: 'var(--space-lg)' }}>
          {[
            { label: 'bg', color: '#f4f5f7', desc: '页面主背景' },
            { label: 'surface', color: '#ffffff', desc: '面板/卡片背景' },
            { label: 'accent', color: '#0f62fe', desc: '品牌强调色' },
            { label: 'canvas', color: '#f8f9fb', desc: '图解画布' },
            { label: 'text', color: '#161616', desc: '主要文字' },
            { label: 'text-sec', color: '#525252', desc: '次要文字' },
            { label: 'border', color: '#d2d2d7', desc: '边框色' },
            { label: 'success', color: '#198038', desc: '成功状态' },
            { label: 'teal', color: '#007d79', desc: '辅助状态' },
            { label: 'danger', color: '#da1e28', desc: '危险状态' },
          ].map((c) => (
            <div key={c.label} style={{ width: 120 }}>
              <div style={{ width: '100%', height: 80, background: c.color, border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-md)', boxShadow: 'var(--shadow-sm)' }} />
              <div style={{ marginTop: 'var(--space-sm)' }}>
                <span style={{ display: 'block', fontWeight: 600, fontSize: 'var(--text-sm)' }}>{c.label}</span>
                <span style={{ display: 'block', fontSize: 10, color: 'var(--color-text-muted)' }}>{c.desc}</span>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Buttons */}
      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Button 按钮</Title>
        <div style={{ display: 'flex', gap: 'var(--space-md)', marginTop: 'var(--space-lg)', alignItems: 'center' }}>
          <Button theme="light" style={{ borderRadius: 'var(--radius-sm)' }}>Default Button</Button>
          <Button theme="solid" style={{ background: 'var(--color-accent)', borderRadius: 'var(--radius-sm)' }}>Primary Action</Button>
          <Button theme="light" type="danger" style={{ borderRadius: 'var(--radius-sm)' }}>Danger Action</Button>
          <Button disabled style={{ borderRadius: 'var(--radius-sm)' }}>Disabled State</Button>
        </div>
      </section>

      {/* Tags / Badges */}
      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Tag 状态徽章</Title>
        <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-lg)', flexWrap: 'wrap' }}>
          <Tag color="grey" style={{ borderRadius: 12 }}>draft 草稿</Tag>
          <Tag color="blue" style={{ borderRadius: 12 }}>submitted 已提交</Tag>
          <Tag color="purple" style={{ borderRadius: 12 }}>ai_reviewing AI 审核中</Tag>
          <Tag color="orange" style={{ borderRadius: 12 }}>human_reviewing 人工审核中</Tag>
          <Tag color="green" style={{ borderRadius: 12 }}>approved 已通过</Tag>
          <Tag color="red" style={{ borderRadius: 12 }}>rejected 已打回</Tag>
        </div>
      </section>

      {/* Card */}
      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Card 卡片面板</Title>
        <Card title="任务卡片示例 · 问答质量标注"
          style={{ marginTop: 'var(--space-lg)', border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-md)', boxShadow: 'var(--shadow-md)' }}
          headerStyle={{ background: '#fafafa', borderBottom: '1px solid var(--color-border-light)' }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>30 题 · 11 个 category · 发布于 2026-05-20</span>
            <Button theme="solid" size="small">进入任务</Button>
          </div>
        </Card>
      </section>

      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Schematic Node</Title>
        <div style={{ marginTop: 'var(--space-lg)', padding: 'var(--space-xl)', border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-lg)', backgroundColor: 'var(--color-canvas)', backgroundImage: 'linear-gradient(var(--color-grid-line) 1px, transparent 1px), linear-gradient(90deg, var(--color-grid-line) 1px, transparent 1px)', backgroundSize: '24px 24px' }}>
          <div style={{ width: 260, border: '1px solid var(--color-node-border)', borderRadius: 'var(--radius-md)', background: 'var(--color-node-bg)', boxShadow: 'var(--shadow-sm)', borderLeft: '3px solid var(--color-rail)' }}>
            <div style={{ padding: 'var(--space-sm) var(--space-md)', borderBottom: '1px solid var(--color-border-light)', background: 'var(--color-panel-header)' }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--color-text-muted)' }}>INPUT.NODE</span>
            </div>
            <div style={{ padding: 'var(--space-md)' }}>
              <strong>annotation_summary</strong>
              <p style={{ margin: 'var(--space-xs) 0 0', color: 'var(--color-text-secondary)', fontSize: 'var(--text-sm)' }}>字段节点、状态 pill、画布网格的标准组合。</p>
            </div>
          </div>
        </div>
      </section>

      <footer style={{ marginTop: 'var(--space-2xl)', textAlign: 'center', paddingBottom: 'var(--space-2xl)' }}>
        <Paragraph style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>
          LabelHub Design System · Typography: Inter · Primary: #0f62fe · Background: #f4f5f7
        </Paragraph>
      </footer>
    </div>
  )
}
