import type { CSSProperties, ErrorInfo, ReactNode } from 'react'
import { Component } from 'react'

type ErrorBoundaryProps = {
  children: ReactNode
  onReset?: () => void
}

type ErrorBoundaryState = {
  error: Error | null
}

export default class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('LabelHub UI crashed', error, info)
  }

  reset = () => {
    this.setState({ error: null })
    this.props.onReset?.()
  }

  render() {
    if (!this.state.error) {
      return this.props.children
    }
    return (
      <main role="alert" aria-label="页面错误" style={fallbackStyle}>
        <h1 style={headingStyle}>页面暂时不可用</h1>
        <p style={copyStyle}>当前视图遇到错误,请重试或返回上一页。</p>
        <button type="button" onClick={this.reset} style={buttonStyle}>重试</button>
      </main>
    )
  }
}

const fallbackStyle = {
  minHeight: '100vh',
  display: 'grid',
  alignContent: 'center',
  justifyItems: 'center',
  gap: 'var(--space-md)',
  padding: 'var(--space-xl)',
  background: 'var(--color-bg)',
  color: 'var(--color-text)',
} satisfies CSSProperties

const headingStyle = {
  margin: 0,
  fontFamily: 'var(--font-heading)',
  fontSize: 'var(--text-h1)',
} satisfies CSSProperties

const copyStyle = {
  margin: 0,
  color: 'var(--color-text-secondary)',
} satisfies CSSProperties

const buttonStyle = {
  minHeight: 40,
  padding: '0 var(--space-lg)',
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--color-surface)',
  cursor: 'pointer',
} satisfies CSSProperties
