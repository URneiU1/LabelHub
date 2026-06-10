// reviewer 模块共享的展示格式化:分数与时间。
// 各 surface 时间粒度不同(纯时间 / 月日时分 / 月日时分秒),用 style 选择;
// 各 style 的空值占位与输出格式与原各处实现保持一致,不改变任何界面显示。

export function formatScore(score: number | null | undefined): string {
  return typeof score === 'number' && Number.isFinite(score) ? String(score) : '-'
}

type TimeStyle = 'time' | 'date-time' | 'date-time-sec'

const TIME_EMPTY: Record<TimeStyle, string> = {
  'time': '--:--',
  'date-time': '--',
  'date-time-sec': '—',
}

export function formatTime(raw: string | null | undefined, style: TimeStyle = 'time'): string {
  if (!raw) {
    return TIME_EMPTY[style]
  }
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return raw
  }
  if (style === 'time') {
    return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
  }
  if (style === 'date-time') {
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
  }
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}
