import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { imageFileUrl } from '../../services/api'
import './Toast.css'

export type ToastNotice = { id: number; text: string; sha1?: string }

const DURATION = 3500

// Uma notificação rápida no canto inferior esquerdo, que some sozinha. Para
// mostrar de novo (mesmo texto), troque o id; a key recomeça o tempo.
export function Toast({ notice }: { notice: ToastNotice | null }) {
  if (!notice) return null
  return createPortal(<ToastItem key={notice.id} notice={notice} />, document.body)
}

function ToastItem({ notice }: { notice: ToastNotice }) {
  const [leaving, setLeaving] = useState(false)
  const [gone, setGone] = useState(false)

  useEffect(() => {
    const leave = window.setTimeout(() => setLeaving(true), DURATION)
    const hide = window.setTimeout(() => setGone(true), DURATION + 260)
    return () => {
      window.clearTimeout(leave)
      window.clearTimeout(hide)
    }
  }, [])

  if (gone) return null
  return (
    <div aria-live="polite" className={leaving ? 'toast toast--leaving' : 'toast'} role="status">
      {notice.sha1 ? <img alt="" className="toast__thumb" src={imageFileUrl(notice.sha1)} /> : <span className="toast__icon">✓</span>}
      <span className="toast__text">{notice.text}</span>
      <button aria-label="Fechar" className="toast__close" onClick={() => setGone(true)} type="button">✕</button>
      <span className="toast__timer" style={{ animationDuration: `${DURATION}ms` }} />
    </div>
  )
}
