import { useEffect, useState } from 'react'
import { Outlet, NavLink, useParams } from 'react-router-dom'
import { clearToken, getCurrentUser, hasAnyRole } from '../api/client'
import {
  DEFAULT_SECTION,
  OWNER_NAV_GROUPS,
  OWNER_SUB_NAV,
  SECTION_DEFAULT_SUB,
  isOwnerSection,
  setOwnerSection,
  setOwnerSubView,
} from '../state/ownerSection'
import { Icon, type IconName } from '../components/Icon'

interface NavItem {
  to: string
  label: string
  roles: string[]
}

const NAV_ITEMS: NavItem[] = [
  { to: '/owner', label: '任务管理', roles: ['owner', 'admin'] },
  { to: '/labeler', label: '标注工作台', roles: ['labeler'] },
  { to: '/reviewer', label: '审核工作台', roles: ['reviewer', 'admin'] },
  { to: '/reviewer/results', label: '审核结果', roles: ['reviewer', 'admin'] },
]

const ROLE_LABEL: Record<string, string> = {
  owner: '任务负责人',
  labeler: '标注员',
  reviewer: '审核员',
  admin: '管理员',
}

// 侧栏导航图标映射(取代空占位方块)
const OWNER_SECTION_ICON: Record<string, IconName> = {
  tasks: 'list',
  template: 'layout',
  dataset: 'database',
  ai: 'sparkle',
  review: 'userCheck',
  stats: 'barChart',
  export: 'download',
}

const ROLE_NAV_ICON: Record<string, IconName> = {
  '/owner': 'list',
  '/labeler': 'edit',
  '/reviewer': 'userCheck',
}

function primaryRole(roles: string[]): string {
  for (const r of ['owner', 'reviewer', 'labeler', 'admin']) {
    if (roles.includes(r)) return r
  }
  return roles[0] ?? ''
}

export default function AppLayout() {
  const params = useParams()
  const user = getCurrentUser()
  const [sideCollapsed, setSideCollapsed] = useState(false)
  const visibleNavItems = NAV_ITEMS.filter((item) => hasAnyRole(item.roles))
  // Owner/admin 的左栏是「工作区」三组分节(对齐 demo SideNav)。每个分节是独立路由页
  // (/owner/:section),侧栏用 NavLink 跳转;分节页内容渲染在 Owner 页(Dashboard)。
  // 因侧栏(shell)与页面(<Outlet/>)分属不同子树,这里把 URL 的 section/sub 同步进
  // 全局 store,Dashboard 订阅后据此渲染对应分节与子页。其它角色仍用各自的路由导航。
  const ownerWorkspace = hasAnyRole(['owner', 'admin'])
  const activeSection = isOwnerSection(params.section) ? params.section : undefined
  const subTabs = activeSection ? OWNER_SUB_NAV[activeSection] : undefined
  // Template Designer is a focus-mode editor: hide the global side nav so the
  // three-pane canvas gets the full width (route is /owner/tasks/:taskId/templates/:templateId).
  const isDesigner = Boolean(params.templateId)

  useEffect(() => {
    if (!ownerWorkspace) return
    setOwnerSection(isOwnerSection(params.section) ? params.section : DEFAULT_SECTION)
    setOwnerSubView(params.sub ?? null)
  }, [ownerWorkspace, params.section, params.sub])

  const otherRoleNavItems = visibleNavItems.filter((item) => item.to !== '/owner')
  const roles = user?.roles ?? []
  const role = primaryRole(roles)
  const roleLabel = ROLE_LABEL[role] ?? role
  const displayName = user?.displayName ?? user?.username ?? '用户'
  const initial = displayName.slice(0, 1)
  const areaLabel = visibleNavItems[0]?.label ?? 'LabelHub'

  return (
    <div className="lh-root">
      <header className="lh-topnav">
        <div className="lh-topnav__brand">
          <span className="lh-topnav__brand-dot" />
          <span>LabelHub</span>
        </div>
        <nav className="lh-topnav__crumbs" aria-label="breadcrumb">
          <strong>{areaLabel}</strong>
        </nav>
        <div className="lh-topnav__right">
          <NavLink to="/auth/login" onClick={clearToken} className="lh-btn lh-btn--ghost lh-btn--sm">
            切换角色
          </NavLink>
          <div className="lh-userchip">
            <div className="lh-userchip__avatar">{initial}</div>
            <span className="lh-userchip__name">{displayName}</span>
            <span className="lh-userchip__role">· {roleLabel}</span>
          </div>
        </div>
      </header>
      <div className={'lh-shell' + (isDesigner ? ' lh-shell--designer' : '') + (sideCollapsed ? ' lh-shell--side-collapsed' : '')}>
        {isDesigner ? null : (
        <aside className="lh-shell__side" aria-label="全局导航">
          <div className="lh-shell__side-head">
            <button
              type="button"
              className="lh-shell__side-toggle"
              aria-label={sideCollapsed ? '展开侧边导航' : '收起侧边导航'}
              aria-expanded={!sideCollapsed}
              onClick={() => setSideCollapsed((current) => !current)}
            >
              <Icon name="list" size={16} />
              <span className="lh-shell__side-toggle-text">{sideCollapsed ? '展开导航' : '收起导航'}</span>
            </button>
          </div>
          {ownerWorkspace ? (
            OWNER_NAV_GROUPS.map((group) => (
              <nav className="lh-side-section" key={group.title} aria-label={group.title}>
                <div className="lh-side-section__title">{group.title}</div>
                {group.items.map(([key, label]) => (
                  <NavLink
                    key={key}
                    to={`/owner/${key}`}
                    aria-label={label}
                    title={label}
                    className={({ isActive }) =>
                      'lh-side-item' + (isActive ? ' lh-side-item--active' : '')
                    }
                  >
                    <Icon
                      name={OWNER_SECTION_ICON[key] ?? 'list'}
                      className="lh-side-item__icon"
                    />
                    <span className="lh-side-item__label">{label}</span>
                  </NavLink>
                ))}
              </nav>
            ))
          ) : (
            <div className="lh-side-section">
              <div className="lh-side-section__title">工作区</div>
              {visibleNavItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end
                  aria-label={item.label}
                  title={item.label}
                  className={({ isActive }) =>
                    'lh-side-item' + (isActive ? ' lh-side-item--active' : '')
                  }
                >
                  <Icon
                    name={ROLE_NAV_ICON[item.to] ?? 'list'}
                    className="lh-side-item__icon"
                  />
                  <span className="lh-side-item__label">{item.label}</span>
                </NavLink>
              ))}
            </div>
          )}
          {ownerWorkspace && otherRoleNavItems.length > 0 ? (
            <div className="lh-side-section">
              <div className="lh-side-section__title">其它</div>
              {otherRoleNavItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  aria-label={item.label}
                  title={item.label}
                  className={({ isActive }) =>
                    'lh-side-item' + (isActive ? ' lh-side-item--active' : '')
                  }
                >
                  <Icon
                    name={ROLE_NAV_ICON[item.to] ?? 'list'}
                    className="lh-side-item__icon"
                  />
                  <span className="lh-side-item__label">{item.label}</span>
                </NavLink>
              ))}
            </div>
          ) : null}
        </aside>
        )}
        <main className="lh-shell__main">
          {subTabs && activeSection ? (
            <nav className="lh-subtabs" aria-label="子页面">
              {SECTION_DEFAULT_SUB[activeSection] ? null : (
                <NavLink
                  to={`/owner/${activeSection}`}
                  end
                  className={({ isActive }) => 'lh-subtab' + (isActive ? ' lh-subtab--active' : '')}
                >
                  全部
                </NavLink>
              )}
              {subTabs.map((tab) => (
                <NavLink
                  key={tab.key}
                  to={`/owner/${activeSection}/${tab.key}`}
                  className={({ isActive }) => 'lh-subtab' + (isActive ? ' lh-subtab--active' : '')}
                >
                  {tab.label}
                </NavLink>
              ))}
            </nav>
          ) : null}
          <Outlet />
        </main>
      </div>
    </div>
  )
}
