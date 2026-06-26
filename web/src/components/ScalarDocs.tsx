import '@scalar/api-reference-react/style.css'
import { ApiReferenceReact } from '@scalar/api-reference-react'

// Wraps Scalar's React renderer. `spec` is the raw OpenAPI text (JSON or YAML);
// Scalar accepts either as inline `content`. Themed to the portal's crimson via
// CSS variables on the wrapper (Scalar reads --scalar-* vars).
export function ScalarDocs({ spec }: { spec: string }) {
  return (
    <div className="scalar-wrap">
      <ApiReferenceReact
        configuration={{
          content: spec,
          hideClientButton: true,
          hideDownloadButton: false,
          theme: 'default',
        }}
      />
    </div>
  )
}
