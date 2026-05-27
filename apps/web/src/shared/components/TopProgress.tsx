export default function TopProgress({ active, label = '加载中' }: { active: boolean, label?: string }) {
  return (
    <div
      className="lh-top-progress"
      data-active={active ? 'true' : 'false'}
      role="progressbar"
      aria-label={label}
      aria-hidden={active ? undefined : 'true'}
    />
  )
}
