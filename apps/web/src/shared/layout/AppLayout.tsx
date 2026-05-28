import { Outlet, NavLink } from 'react-router-dom'
import { Layout, Nav, Typography } from '@douyinfe/semi-ui'
import { clearToken, hasAnyRole } from '../api/client'

const { Header, Sider, Content } = Layout
const { Text } = Typography

const navItems = [
  { to: '/owner', label: '任务负责人', roles: ['owner', 'admin'] },
  { to: '/labeler', label: '标注工作台', roles: ['labeler'] },
  { to: '/reviewer', label: '审核中心', roles: ['reviewer', 'owner', 'admin'] },
]

export default function AppLayout() {
  const visibleNavItems = navItems.filter((item) => hasAnyRole(item.roles))

  return (
    <Layout style={{ minHeight: '100vh', background: 'var(--color-bg)' }}>
      <Sider style={{ background: 'var(--color-surface)', borderRight: '1px solid var(--color-border-light)', zIndex: 10, width: 240 }}>
        <div style={{ padding: 'var(--space-lg) var(--space-xl)', display: 'flex', alignItems: 'center', gap: 'var(--space-md)', borderBottom: '1px solid var(--color-border-light)' }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 2, width: 20, height: 20 }}>
            <div style={{ background: 'var(--color-accent)', borderRadius: 2 }} />
            <div style={{ background: 'var(--color-border)', borderRadius: 2 }} />
            <div style={{ background: 'var(--color-border)', borderRadius: 2 }} />
            <div style={{ background: 'var(--color-accent)', borderRadius: 2 }} />
          </div>
          <Text strong style={{ fontFamily: 'var(--font-heading)', fontSize: '1.125rem' }}>LabelHub</Text>
        </div>
        <Nav style={{ padding: 'var(--space-md) 0', background: 'transparent' }}>
          {visibleNavItems.map((item) => (
            <NavLink key={item.to} to={item.to} style={navLinkStyle}>{item.label}</NavLink>
          ))}
        </Nav>
      </Sider>
      <Layout>
        <Header style={{ background: 'var(--color-surface)', borderBottom: '1px solid var(--color-border-light)', padding: '0 var(--space-xl)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', height: 48 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-md)' }}>
            <Text style={{ fontFamily: 'var(--font-heading)', fontSize: 'var(--text-sm)', fontWeight: 600, color: 'var(--color-text)' }}>Production Console</Text>
            <div style={{ width: 1, height: 16, background: 'var(--color-border-light)' }} />
            <Text style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', color: 'var(--color-text-muted)' }}>SYS.CTRL.01</Text>
          </div>
          <NavLink to="/auth/login" onClick={clearToken} style={switchRoleStyle}>切换角色</NavLink>
        </Header>
        <Content style={{ padding: 'var(--space-xl)', overflow: 'auto' }}>
          <div style={{ maxWidth: 1440, margin: '0 auto' }}>
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  )
}

const navLinkStyle = ({ isActive }: { isActive: boolean }): React.CSSProperties => ({
  display: 'flex',
  alignItems: 'center',
  padding: 'var(--space-sm) var(--space-xl)',
  color: isActive ? 'var(--color-accent)' : 'var(--color-text-secondary)',
  background: isActive ? 'var(--color-info-bg)' : 'transparent',
  borderLeft: isActive ? '3px solid var(--color-accent)' : '3px solid transparent',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-sm)',
  fontWeight: isActive ? 600 : 500,
  textDecoration: 'none',
  marginBottom: 'var(--space-xs)',
  transition: 'all var(--duration-fast) var(--ease-out)',
})

const switchRoleStyle: React.CSSProperties = {
  color: 'var(--color-accent)',
  fontFamily: 'var(--font-body)',
  fontSize: 'var(--text-sm)',
  fontWeight: 600,
  textDecoration: 'none',
}
