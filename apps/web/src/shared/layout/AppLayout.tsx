import { Outlet, NavLink } from 'react-router-dom'
import { Layout, Nav, Typography } from '@douyinfe/semi-ui'

const { Header, Sider, Content } = Layout
const { Text } = Typography

export default function AppLayout() {
  return (
    <Layout style={{ minHeight: '100vh', background: 'var(--color-bg)' }}>
      <Sider style={{ background: 'var(--color-surface)', borderRight: '1px solid var(--color-border)' }}>
        <div style={{ padding: 'var(--space-lg)', borderBottom: '1px solid var(--color-border)' }}>
          <Text strong style={{ fontFamily: 'var(--font-heading)', fontSize: '1.125rem' }}>LabelHub</Text>
        </div>
        <Nav style={{ padding: 'var(--space-sm) 0' }}>
          <NavLink to="/owner" style={navLinkStyle}>任务负责人</NavLink>
          <NavLink to="/labeler" style={navLinkStyle}>标注工作台</NavLink>
          <NavLink to="/reviewer" style={navLinkStyle}>审核中心</NavLink>
        </Nav>
      </Sider>
      <Layout>
        <Header style={{ background: 'var(--color-surface)', borderBottom: '1px solid var(--color-border)', padding: '0 var(--space-lg)', display: 'flex', alignItems: 'center' }}>
          <Text style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-h2)' }}>LabelHub · 数据标注平台</Text>
        </Header>
        <Content style={{ padding: 'var(--space-lg)' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

const navLinkStyle = (): React.CSSProperties => ({
  display: 'block',
  padding: 'var(--space-sm) var(--space-lg)',
  color: 'var(--color-text)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-base)',
  textDecoration: 'none',
})
