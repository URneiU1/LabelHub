import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ErrorBoundary from './ErrorBoundary'

function Crash() {
  throw new Error('render failed')
}

describe('ErrorBoundary', () => {
  let consoleError: ReturnType<typeof vi.spyOn>
  const preventExpectedError = (event: ErrorEvent) => event.preventDefault()

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    window.addEventListener('error', preventExpectedError)
  })

  afterEach(() => {
    window.removeEventListener('error', preventExpectedError)
    consoleError.mockRestore()
  })

  it('renders fallback UI and can reset after a child render crash', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [crashed, setCrashed] = useState(true)
      return (
        <ErrorBoundary onReset={() => setCrashed(false)}>
          {crashed ? <Crash /> : <div>Recovered view</div>}
        </ErrorBoundary>
      )
    }

    render(<Harness />)

    expect(screen.getByRole('alert', { name: '页面错误' })).toBeInTheDocument()
    expect(screen.getByText('页面暂时不可用')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '重试' }))

    expect(screen.getByText('Recovered view')).toBeInTheDocument()
  })
})
