import { useRef, useState, useCallback, useEffect } from 'react'
import { transcribeAudio } from '../../api'
import { BAR_COUNT } from './AttachmentCard'

export function useAudioRecorder({
  accessTokenRef,
  valueRef,
  onChangeRef,
  onAsrErrorRef,
}: {
  accessTokenRef: React.RefObject<string | undefined>
  valueRef: React.RefObject<string>
  onChangeRef: React.RefObject<(val: string) => void>
  onAsrErrorRef: React.RefObject<((err: unknown) => void) | undefined>
}) {
  const mediaRecorderRef = useRef<MediaRecorder | null>(null)
  const audioChunksRef = useRef<Blob[]>([])
  const analyserRef = useRef<AnalyserNode | null>(null)
  const waveformHistoryRef = useRef<number[]>(Array(BAR_COUNT).fill(0))
  const animFrameRef = useRef<number>(0)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const discardRef = useRef(false)

  const [isRecording, setIsRecording] = useState(false)
  const [isTranscribing, setIsTranscribing] = useState(false)
  const [recordingSeconds, setRecordingSeconds] = useState(0)
  const [waveformBars, setWaveformBars] = useState<number[]>(Array(BAR_COUNT).fill(0))

  useEffect(() => {
    return () => {
      cancelAnimationFrame(animFrameRef.current)
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [])

  const startRecording = useCallback(async () => {
    if (isRecording || isTranscribing || !accessTokenRef.current) return

    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch {
      return
    }

    const audioCtx = new AudioContext()
    const analyser = audioCtx.createAnalyser()
    analyser.fftSize = 1024
    analyser.smoothingTimeConstant = 0.5
    audioCtx.createMediaStreamSource(stream).connect(analyser)
    analyserRef.current = analyser
    waveformHistoryRef.current = Array(BAR_COUNT).fill(0)

    const dataArray = new Float32Array(analyser.fftSize)
    let lastSample = 0
    const tick = () => {
      analyser.getFloatTimeDomainData(dataArray)
      const now = performance.now()
      if (now - lastSample >= 80) {
        lastSample = now
        let sum = 0
        for (let i = 0; i < dataArray.length; i++) sum += dataArray[i] ** 2
        const rms = Math.sqrt(sum / dataArray.length)
        const history = waveformHistoryRef.current
        history.shift()
        history.push(Math.min(1, rms * 8))
        setWaveformBars([...history])
      }
      animFrameRef.current = requestAnimationFrame(tick)
    }
    animFrameRef.current = requestAnimationFrame(tick)

    setRecordingSeconds(0)
    timerRef.current = setInterval(() => setRecordingSeconds((s) => s + 1), 1000)

    const recorder = new MediaRecorder(stream)
    mediaRecorderRef.current = recorder
    audioChunksRef.current = []
    discardRef.current = false

    recorder.ondataavailable = (e) => {
      if (e.data.size > 0) audioChunksRef.current.push(e.data)
    }

    recorder.onstop = async () => {
      cancelAnimationFrame(animFrameRef.current)
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
      try { audioCtx.close() } catch { /* ignore */ }
      stream.getTracks().forEach((t) => t.stop())
      setIsRecording(false)

      if (discardRef.current) {
        discardRef.current = false
        audioChunksRef.current = []
        setWaveformBars(Array(BAR_COUNT).fill(0))
        return
      }

      const token = accessTokenRef.current
      if (!token || audioChunksRef.current.length === 0) return

      const lang = navigator.language?.split('-')[0] ?? undefined

      const blob = new Blob(audioChunksRef.current, { type: 'audio/webm' })
      setIsTranscribing(true)
      try {
        const result = await transcribeAudio(token, blob, 'audio.webm', lang)
        if (result.text) {
          const prev = valueRef.current
          onChangeRef.current(prev ? `${prev} ${result.text}` : result.text)
        }
      } catch (err) {
        onAsrErrorRef.current?.(err)
      } finally {
        setIsTranscribing(false)
        setWaveformBars(Array(BAR_COUNT).fill(0))
      }
    }

    recorder.start()
    setIsRecording(true)
  }, [isRecording, isTranscribing, accessTokenRef, valueRef, onChangeRef, onAsrErrorRef])

  const stopAndTranscribe = useCallback(() => {
    discardRef.current = false
    mediaRecorderRef.current?.stop()
  }, [])

  const cancelRecording = useCallback(() => {
    discardRef.current = true
    mediaRecorderRef.current?.stop()
  }, [])

  return { isRecording, isTranscribing, recordingSeconds, waveformBars, startRecording, stopAndTranscribe, cancelRecording }
}
