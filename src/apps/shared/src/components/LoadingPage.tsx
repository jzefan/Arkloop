import { AuthLayout, SpinnerIcon } from './auth-ui'

type Props = {
  label: string
  brandLabel?: string
}

export function LoadingPage({ label, brandLabel = 'Arkloop' }: Props) {
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
      </div>
    </AuthLayout>
  )
}
