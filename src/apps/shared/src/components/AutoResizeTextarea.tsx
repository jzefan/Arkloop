import { forwardRef, useCallback } from 'react'
import type { CSSProperties, TextareaHTMLAttributes } from 'react'
import { useAutoResizeTextarea } from '../text/useAutoResizeTextarea'

type Props = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  font?: string
  lineHeight?: number
  minRows?: number
  maxHeight?: number
  autoResize?: boolean
}

export const AutoResizeTextarea = forwardRef<HTMLTextAreaElement, Props>(function AutoResizeTextarea(
  {
    value,
    font,
    lineHeight,
    minRows = 1,
    maxHeight,
    autoResize = true,
    style,
    rows,
    ...rest
  },
  forwardedRef,
) {
  const textValue = value == null ? '' : String(value)
  const { ref, height, overflowY, recompute } = useAutoResizeTextarea({
    value: textValue,
    font,
    lineHeight,
    minRows,
    maxHeight: autoResize ? maxHeight : undefined,
  })
  const setRefs = useCallback((node: HTMLTextAreaElement | null) => {
    ref.current = node
    if (!forwardedRef) return
    if (typeof forwardedRef === 'function') {
      forwardedRef(node)
      return
    }
    forwardedRef.current = node
  }, [forwardedRef, ref])

  const mergedStyle: CSSProperties = {
    ...style,
    height: autoResize && height != null ? `${height}px` : style?.height,
    overflowY: autoResize ? overflowY : style?.overflowY,
  }

  return (
    <textarea
      {...rest}
      ref={setRefs}
      value={value}
      rows={rows ?? minRows}
      style={mergedStyle}
      onInput={(event) => {
        recompute()
        rest.onInput?.(event)
      }}
    />
  )
})
