export interface AppError {
  message: string
  traceId?: string
  code?: string
}

interface Props {
  error: AppError
}

export function ErrorCallout({ error }: Props) {
  return (
    <div
      style={{
        padding: '12px 16px',
        borderRadius: '8px',
        background: 'rgba(220, 53, 69, 0.1)',
        border: '1px solid rgba(220, 53, 69, 0.3)',
        color: 'rgb(220, 53, 69)',
        fontSize: '14px',
        marginBottom: '12px',
      }}
    >
      <div style={{ fontWeight: 500, marginBottom: '4px' }}>Error</div>
      <div style={{ fontSize: '13px', lineHeight: 1.5 }}>
        {error.message}
        {error.code && (
          <div style={{ marginTop: '4px', fontSize: '12px', opacity: 0.8 }}>
            Code: {error.code}
          </div>
        )}
        {error.traceId && (
          <div style={{ marginTop: '4px', fontSize: '12px', opacity: 0.8 }}>
            Trace ID: {error.traceId}
          </div>
        )}
      </div>
    </div>
  )
}
