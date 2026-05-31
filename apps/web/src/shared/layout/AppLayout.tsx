import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import { clearToken, getCurrentUser, hasAnyRole } from '../api/client'
import {
  OWNER_NAV_GROUPS,
  OWNER_SUB_NAV,
  setOwnerSection,
  setOwnerSubTarget,
  useOwnerSection,
  useOwnerSubTarget,
} from '../state/ownerSection'

interface NavItem {
  to: string
  label: string
  roles: string[]
}

const NAV_ITEMS: NavItem[] = [
  { to: '/owner', label: '任务管理', roles: ['owner', 'admin'] },
  { to: '/labeler', label: '标注工作台', roles: ['labeler'] },
  { to: '/reviewer', label: '审核中心', roles: ['reviewer', 'admin'] },
]

const ROLE_LABEL: Record<string, string> = {
  owner: '任务负责人',
  labeler: '标注员',
  reviewer: '审核员',
  admin: '管理员',
}

function primaryRole(roles: string[]): string {
  for (const r of ['owner', 'reviewer', 'labeler', 'admin']) {
    if (roles.includes(r)) return r
  }
  return roles[0] ?? ''
}

export default function AppLayout() {
  const navigate = useNavigate()
  const ownerSection = useOwnerSection()
  const ownerSubTarget = useOwnerSubTarget()
  const user = getCurrentUser()
  const visibleNavItems = NAV_ITEMS.filter((item) => hasAnyRole(item.roles))
  // Owner/admin 的左栏是「工作区」三组分节(对齐 demo SideNav);分节由 ownerSection store 驱动,
  // 跨子树同步到 Owner 页(Dashboard)。其它角色仍用各自的路由导航。
  const ownerWorkspace = hasAnyRole(['owner', 'admin'])
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
      <div className="lh-shell">
        <aside className="lh-shell__side">
          {ownerWorkspace ? (
            OWNER_NAV_GROUPS.map((group) => (
              <nav className="lh-side-section" key={group.title} aria-label={group.title}>
                <div className="lh-side-section__title">{group.title}</div>
                {group.items.map(([key, label]) => {
                  const isActive = ownerSection === key
                  const subItems = OWNER_SUB_NAV[key]
                  return (
                    <div key={key}>
                      <button
                        type="button"
                        className={'lh-side-item' + (isActive ? ' lh-side-item--active' : '')}
                        aria-current={isActive ? 'page' : undefined}
                        aria-expanded={subItems ? isActive : undefined}
                        onClick={() => {
                          setOwnerSection(key)
                          navigate('/owner')
                        }}
                      >
                        <span className="lh-side-item__icon" />
                        {label}
                        {subItems ? <span className="lh-side-item__caret" aria-hidden="true" /> : null}
                      </button>
                      {isActive && subItems ? (
                        <div className="lh-side-subnav" role="group" aria-label={label + ' 子菜单'}>
                          {subItems.map((sub) => (
                            <button
                              key={sub.anchor}
                              type="button"
                              className={
                                'lh-side-subitem' +
                                (ownerSubTarget?.anchor === sub.anchor ? ' lh-side-subitem--active' : '')
                              }
                              onClick={() => {
                                setOwnerSection(key)
                                navigate('/owner')
                                setOwnerSubTarget(sub.anchor)
                              }}
                            >
                              {sub.label}
                            </button>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  )
                })}
              </nav>
            ))
          ) : (
            <div className="lh-side-section">
              <div className="lh-side-section__title">工作区</div>
              {visibleNavItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    'lh-side-item' + (isActive ? ' lh-side-item--active' : '')
                  }
                >
                  <span className="lh-side-item__icon" />
                  {item.label}
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
                  className={({ isActive }) =>
                    'lh-side-item' + (isActive ? ' lh-side-item--active' : '')
                  }
                >
                  <span className="lh-side-item__icon" />
                  {item.label}
                </NavLink>
              ))}
            </div>
          ) : null}
        </aside>
        <main className="lh-shell__main">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
