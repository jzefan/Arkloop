import { useRef, useState, useCallback, useEffect } from 'react'
import { hasTransferFiles, extractFilesFromTransfer, isEditableElement } from './AttachmentCard'

export function useAttachments({
  onAttachFiles,
  textareaRef,
}: {
  onAttachFiles?: (files: File[]) => void
  textareaRef: React.RefObject<HTMLTextAreaElement | null>
}) {
  const dragDepthRef = useRef(0)
  const lastPasteRef = useRef(0)
  const pasteProcessingRef = useRef(false)
  const [isFileDragging, setIsFileDragging] = useState(false)

  const handleAttachTransfer = useCallback((dataTransfer?: DataTransfer | null) => {
    if (pasteProcessingRef.current) return false
    const files = extractFilesFromTransfer(dataTransfer)
    if (files.length === 0 || !onAttachFiles) return false
    pasteProcessingRef.current = true
    onAttachFiles(files)
    textareaRef.current?.focus()
    requestAnimationFrame(() => { pasteProcessingRef.current = false })
    return true
  }, [onAttachFiles, textareaRef])

  // window-level drag/drop
  useEffect(() => {
    if (!onAttachFiles) return

    const resetDragState = () => {
      dragDepthRef.current = 0
      setIsFileDragging(false)
    }

    const handleDragEnter = (e: DragEvent) => {
      if (!hasTransferFiles(e.dataTransfer)) return
      e.preventDefault()
      dragDepthRef.current += 1
      setIsFileDragging(true)
    }

    const handleDragOver = (e: DragEvent) => {
      if (!hasTransferFiles(e.dataTransfer)) return
      e.preventDefault()
      if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
      setIsFileDragging(true)
    }

    const handleDragLeave = (e: DragEvent) => {
      if (dragDepthRef.current === 0 && !hasTransferFiles(e.dataTransfer)) return
      e.preventDefault()
      dragDepthRef.current = Math.max(0, dragDepthRef.current - 1)
      if (dragDepthRef.current === 0) {
        setIsFileDragging(false)
      }
    }

    const handleDrop = (e: DragEvent) => {
      if (dragDepthRef.current === 0 && !hasTransferFiles(e.dataTransfer)) return
      e.preventDefault()
      handleAttachTransfer(e.dataTransfer)
      resetDragState()
    }

    const handleWindowBlur = () => {
      resetDragState()
    }

    window.addEventListener('dragenter', handleDragEnter)
    window.addEventListener('dragover', handleDragOver)
    window.addEventListener('dragleave', handleDragLeave)
    window.addEventListener('drop', handleDrop)
    window.addEventListener('blur', handleWindowBlur)

    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('drop', handleDrop)
      window.removeEventListener('blur', handleWindowBlur)
      resetDragState()
    }
  }, [handleAttachTransfer, onAttachFiles])

  // document-level paste for files (outside textarea)
  useEffect(() => {
    if (!onAttachFiles) return
    const handlePaste = (e: ClipboardEvent) => {
      if (e.target === textareaRef.current) return
      if (isEditableElement(e.target)) return
      if (!hasTransferFiles(e.clipboardData)) return
      if (pasteProcessingRef.current) { e.preventDefault(); return }
      const now = Date.now()
      if (now - lastPasteRef.current < 1000) { e.preventDefault(); return }
      lastPasteRef.current = now
      if (!handleAttachTransfer(e.clipboardData)) return
      e.preventDefault()
    }
    document.addEventListener('paste', handlePaste)
    return () => document.removeEventListener('paste', handlePaste)
  }, [handleAttachTransfer, onAttachFiles, textareaRef])

  return { isFileDragging, handleAttachTransfer, pasteProcessingRef, lastPasteRef }
}
