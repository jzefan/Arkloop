import { AuthLayout, SpinnerIcon } from './auth-ui'
import { PRODUCT_BRAND_NAME } from '../brand'

type Props = {
  label: string
  brandLabel?: string
  error?: { title: string; message: string }
  onRetry?: () => void
  retryLabel?: string
}

export function LoadingPage({ label, brandLabel = PRODUCT_BRAND_NAME, error, onRetry, retryLabel }: Props) {
  return (
    <AuthLayout>
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '14px',
          minHeight: '240px',
          textAlign: 'center',
        }}
      >
        <div
          style={{
            fontSize: '28px',
            fontWeight: 500,
            color: 'var(--c-text-primary)',
            lineHeight: 1,
          }}
        >
          {brandLabel}
        </div>
        {error ? (
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '10px', maxWidth: '320px' }}>
            <div style={{ fontSize: '14px', fontWeight: 500, color: 'var(--c-status-error-text, var(--c-status-error))' }}>
              {error.title}
            </div>
            <div style={{ fontSize: '13px', color: 'var(--c-text-secondary)' }}>
              {error.message}
            </div>
            {onRetry && (
              <button
                onClick={onRetry}
                style={{
                  background: 'var(--c-btn-bg)',
                  color: 'var(--c-btn-text)',
                  borderRadius: '8px',
                  padding: '8px 16px',
                  fontSize: '13px',
                  fontWeight: 500,
                  border: 'none',
                  cursor: 'pointer',
                  marginTop: '4px',
                }}
              >
                {retryLabel ?? '重新连接'}
              </button>
            )}
          </div>
        ) : (
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '8px',
              fontSize: '14px',
              fontWeight: 500,
              color: 'var(--c-placeholder)',
            }}
          >
            <SpinnerIcon />
            <span>{label}</span>
          </div>
        )}
      </div>
    </AuthLayout>
  )
}
