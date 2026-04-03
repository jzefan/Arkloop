import { useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { AnimatedCheck } from './AnimatedCheck'

type Phase = 'idle' | 'spinning' | 'done'

type Props = {
  onRefresh: () => void
  disabled?: boolean
  size?: number
  className?: string
  tooltip?: string
}

const SPIN_KEYFRAME = `@keyframes spin-once { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }`

let styleInjected = false
function ensureStyle() {
  if (styleInjected) return
  const el = document.createElement('style')
  el.textContent = SPIN_KEYFRAME
  document.head.appendChild(el)
  styleInjected = true
}

export function RefreshIconButton({ onRefresh, disabled = false, size = 16, className, tooltip = 'Retry' }: Props) {
  const [phase, setPhase] = useState<Phase>('idle')
  const [hovered, setHovered] = useState(false)

  const interactive = !disabled && phase === 'idle'

  const handleClick = () => {
    if (!interactive) return
    ensureStyle()
    onRefresh()
    setPhase('spinning')
  }

  const handleAnimationEnd = () => {
    setPhase('done')
    setTimeout(() => setPhase('idle'), 1500)
  }

  const handleMouseEnter = () => setHovered(true)
  const handleMouseLeave = () => {
    setHovered(false)
  }

  const iconSpanStyle: React.CSSProperties = phase === 'spinning'
    ? {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        animation: 'spin-once 200ms ease-out forwards',
      }
    : {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }

  const showTooltip = hovered && phase === 'idle' && !disabled

  return (
    <span style={{ position: 'relative', display: 'inline-flex' }}>
      <button
        onClick={handleClick}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        disabled={disabled}
        className={className}
      >
        <span style={iconSpanStyle} onAnimationEnd={phase === 'spinning' ? handleAnimationEnd : undefined}>
          {phase === 'done'
            ? <AnimatedCheck size={size} />
            : <RefreshCw size={size} />
          }
        </span>
      </button>
      <span
        style={{
          position: 'absolute',
          top: '100%',
          left: '50%',
          transform: showTooltip
            ? 'translateX(-50%) translateY(0px)'
            : 'translateX(-50%) translateY(-3px)',
          marginTop: '3px',
          fontSize: '11px',
          fontWeight: 500,
          color: 'rgba(255,255,255,0.9)',
          background: 'rgba(0,0,0,0.75)',
          borderRadius: '5px',
          padding: '2px 7px',
          whiteSpace: 'nowrap',
          opacity: showTooltip ? 1 : 0,
          transition: 'opacity 120ms ease, transform 120ms ease',
          pointerEvents: 'none',
          userSelect: 'none',
          zIndex: 20,
        }}
      >
        {tooltip}
      </span>
    </span>
  )
}
