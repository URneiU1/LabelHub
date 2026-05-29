import { useState } from 'react'
import { Button, Card, Form, Modal, Table, Tag, Typography } from '@douyinfe/semi-ui'
import StatusBadge from '../../shared/components/StatusBadge'

const { Title, Paragraph } = Typography

export default function StyleGuide() {
  const [modalVisible, setModalVisible] = useState(false)
  const tableColumns = [
    { title: '任务', dataIndex: 'task' },
    { title: '状态', dataIndex: 'status' },
    { title: 'SLA', dataIndex: 'sla' },
  ]
  const tableData = [
    { key: '1', task: 'qa_quality · 电商标题质检', status: <Tag color="green">published</Tag>, sla: '02:14:00' },
    { key: '2', task: 'image_review · 图文安全', status: <Tag color="orange">human_reviewing</Tag>, sla: '00:42:18' },
    { key: '3', task: 'llm_eval · Prompt dry-run', status: <Tag color="blue">ai_reviewing</Tag>, sla: '01:06:44' },
  ]

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: 'var(--space-2xl) var(--space-xl)', background: 'var(--color-bg)', minHeight: '100vh' }}>
      <div style={{ background: 'var(--color-surface)', padding: 'var(--space-2xl)', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={1} style={{ fontFamily: 'var(--font-heading)', fontWeight: 700, letterSpacing: 0 }}>Editorial Console Style Guide</Title>
        <Paragraph style={{ color: 'var(--color-text-secondary)', fontFamily: 'var(--font-body)', fontSize: 'var(--text-h2)' }}>
          LabelHub Editorial 风格令牌 · 高密度控制台 · 稳定状态色 · 共享状态组件
        </Paragraph>
      </div>

      {/* Color Palette */}
      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>色彩 Palette</Title>
        <div style={{ display: 'flex', gap: 'var(--space-lg)', flexWrap: 'wrap', marginTop: 'var(--space-lg)' }}>
          {[
            { label: 'bg', color: 'var(--color-bg)', desc: '页面主背景' },
            { label: 'surface', color: 'var(--color-surface)', desc: '面板/卡片背景' },
            { label: 'accent', color: 'var(--color-accent)', desc: '品牌强调色' },
            { label: 'canvas', color: 'var(--color-canvas)', desc: '工作区画布' },
            { label: 'text', color: 'var(--color-text)', desc: '主要文字' },
            { label: 'text-sec', color: 'var(--color-text-secondary)', desc: '次要文字' },
            { label: 'border', color: 'var(--color-border)', desc: '边框色' },
            { label: 'success', color: 'var(--color-success)', desc: '成功状态' },
            { label: 'teal', color: 'var(--color-teal)', desc: '辅助状态' },
            { label: 'danger', color: 'var(--color-danger)', desc: '危险状态' },
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
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>StatusBadge 状态徽章</Title>
        <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-lg)', flexWrap: 'wrap', alignItems: 'center' }}>
          {['draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'approved', 'rejected', 'revising'].map((status) => (
            <StatusBadge key={status} status={status} />
          ))}
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
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Form 表单</Title>
        <Form layout="vertical" style={{ maxWidth: 520, marginTop: 'var(--space-lg)' }}>
          <Form.Input field="task_name" label="任务名称" initValue="qa_quality · 电商标题质检" />
          <Form.Select field="review_mode" label="审核模式" initValue="ai_then_human" optionList={[
            { label: 'AI 预审 + 人工复核', value: 'ai_then_human' },
            { label: '仅人工审核', value: 'human_only' },
          ]} />
          <Form.TextArea field="baseline" label="Baseline 描述" initValue="商品标题需覆盖核心事实、类目和安全要求。" rows={3} />
        </Form>
      </section>

      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Table 表格</Title>
        <Table
          columns={tableColumns}
          dataSource={tableData}
          pagination={false}
          style={{ marginTop: 'var(--space-lg)', border: '1px solid var(--color-border-light)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}
        />
      </section>

      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Modal 弹窗</Title>
        <Paragraph style={{ color: 'var(--color-text-secondary)' }}>标准确认弹窗用于发布、批量审核和危险操作。</Paragraph>
        <Button theme="solid" onClick={() => setModalVisible(true)} style={{ background: 'var(--color-accent)', borderRadius: 'var(--radius-sm)' }}>打开确认弹窗</Button>
        <Modal
          title="发布任务确认"
          visible={modalVisible}
          onCancel={() => setModalVisible(false)}
          onOk={() => setModalVisible(false)}
          okText="确认发布"
          cancelText="取消"
        >
          <p>确认后任务将进入 Labeler 广场，并按当前 AI 预审配置处理新提交。</p>
        </Modal>
      </section>

      <section style={{ marginTop: 'var(--space-2xl)', background: 'var(--color-surface)', padding: 'var(--space-xl)', borderRadius: 'var(--radius-lg)', border: '1px solid var(--color-border-light)' }}>
        <Title heading={2} style={{ fontFamily: 'var(--font-heading)', fontWeight: 600 }}>Canvas Node</Title>
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
          LabelHub Design System · Typography: Georgia/serif + system sans · Primary: #b8442e · Background: #f7f7f4
        </Paragraph>
      </footer>
    </div>
  )
}
