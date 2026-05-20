import { Button, Card, Tag, Table, Typography } from '@douyinfe/semi-ui'

const { Title, Paragraph } = Typography

interface StyleTableRow {
  id: string
  category: string
  status: string
}

export default function StyleGuide() {
  return (
    <div style={{ maxWidth: 960, margin: '0 auto', padding: 'var(--space-2xl) var(--space-lg)', background: 'var(--color-bg)' }}>
      <Title heading={1} style={{ fontFamily: 'var(--font-heading)' }}>Editorial Style Guide</Title>
      <Paragraph style={{ color: 'var(--color-text-secondary)', fontFamily: 'var(--font-body)' }}>
        LabelHub Editorial / Magazine 风格令牌 · 浅米色背景 · Serif 标题 · 黑色细线 · 暖红强调
      </Paragraph>

      {/* Color Palette */}
      <section style={{ marginTop: 'var(--space-2xl)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)' }}>色彩 Palette</Title>
        <div style={{ display: 'flex', gap: 'var(--space-md)', flexWrap: 'wrap', marginTop: 'var(--space-md)' }}>
          {[
            { label: 'bg', color: '#F5F1E8' },
            { label: 'text', color: '#1A1A1A' },
            { label: 'accent', color: '#C73E1D' },
            { label: 'surface', color: '#FFFFFF' },
            { label: 'border', color: '#1A1A1A' },
            { label: 'success', color: '#2D6A4F' },
          ].map((c) => (
            <div key={c.label} style={{ textAlign: 'center' }}>
              <div style={{ width: 64, height: 64, background: c.color, border: '1px solid var(--color-border)', borderRadius: 'var(--radius-sm)' }} />
              <span style={{ display: 'block', marginTop: 'var(--space-xs)', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>{c.label}</span>
            </div>
          ))}
        </div>
      </section>

      {/* Buttons */}
      <section style={{ marginTop: 'var(--space-2xl)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)' }}>Button 按钮</Title>
        <div style={{ display: 'flex', gap: 'var(--space-md)', marginTop: 'var(--space-md)', alignItems: 'center' }}>
          <Button>Default</Button>
          <Button theme="solid" style={{ background: 'var(--color-accent)', borderColor: 'var(--color-accent)' }}>Primary</Button>
          <Button disabled>Disabled</Button>
        </div>
      </section>

      {/* Tags / Badges */}
      <section style={{ marginTop: 'var(--space-2xl)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)' }}>Tag 状态徽章</Title>
        <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-md)', flexWrap: 'wrap' }}>
          <Tag color="grey">draft 草稿</Tag>
          <Tag color="blue">submitted 已提交</Tag>
          <Tag color="purple">ai_reviewing AI 审核中</Tag>
          <Tag color="orange">human_reviewing 人工审核中</Tag>
          <Tag color="green">approved 已通过</Tag>
          <Tag color="red">rejected 已打回</Tag>
          <Tag color="yellow">revising 修订中</Tag>
        </div>
      </section>

      {/* Card */}
      <section style={{ marginTop: 'var(--space-2xl)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)' }}>Card 卡片</Title>
        <Card title="任务卡片示例 · 问答质量标注"
          style={{ marginTop: 'var(--space-md)', border: '1px solid var(--color-border)', borderRadius: 'var(--radius-sm)', boxShadow: 'var(--shadow-sm)' }}
          headerStyle={{ fontFamily: 'var(--font-heading)', borderBottom: '1px solid var(--color-border)' }}
        >
          30 题 · 11 个 category · 发布于 2026-05-20
        </Card>
      </section>

      {/* Table */}
      <section style={{ marginTop: 'var(--space-2xl)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)' }}>Table 数据表</Title>
        <Table
          columns={[
            { title: '题目 ID', dataIndex: 'id' },
            { title: '类别', dataIndex: 'category' },
            { title: '状态', dataIndex: 'status', render: (_: unknown, r: StyleTableRow) => <Tag color={r.status === 'approved' ? 'green' : 'grey'}>{r.status}</Tag> },
          ]}
          dataSource={[
            { id: 'Q0001', category: '知识问答', status: 'approved' },
            { id: 'Q0002', category: '代码生成', status: 'pending' },
          ]}
          style={{ marginTop: 'var(--space-md)' }}
        />
      </section>

      {/* Modal Placeholder */}
      <section style={{ marginTop: 'var(--space-2xl)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)' }}>Form 表单</Title>
        <div style={{ border: '1px solid var(--color-border-light)', padding: 'var(--space-lg)', marginTop: 'var(--space-md)', background: 'var(--color-surface)' }}>
          <p style={{ fontFamily: 'var(--font-body)', color: 'var(--color-text-muted)', margin: 0 }}>
            Formily Renderer 将在 Sprint 1 接入,此处为占位。
          </p>
        </div>
      </section>

      <footer style={{ marginTop: 'var(--space-2xl)', paddingTop: 'var(--space-lg)', borderTop: '1px solid var(--color-border)' }}>
        <Paragraph style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>
          Fonts: IBM Plex Serif (headings) + Inter (body) · Primary: #C73E1D · Background: #F5F1E8
        </Paragraph>
      </footer>
    </div>
  )
}
