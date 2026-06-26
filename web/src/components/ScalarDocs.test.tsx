import { it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ScalarDocs } from './ScalarDocs'

// Scalar renders nothing meaningful in jsdom and pulls a large bundle; mock it
// and assert ScalarDocs hands it our spec content.
vi.mock('@scalar/api-reference-react', () => ({
  ApiReferenceReact: ({ configuration }: { configuration: { content: string } }) => (
    <div data-testid="scalar" data-content={configuration.content} />
  ),
}))

it('passes the spec to ApiReferenceReact as content', () => {
  render(<ScalarDocs spec='{"openapi":"3.0.0"}' />)
  expect(screen.getByTestId('scalar')).toHaveAttribute('data-content', '{"openapi":"3.0.0"}')
})
