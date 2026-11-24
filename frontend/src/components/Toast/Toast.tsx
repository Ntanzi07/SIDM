import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { imageFileUrl } from '../../services/api'
import { dismiss, subscribeToasts, type ToastItem, type ToastKind } from '../../services/toast'
import './Toast.css'

const DURATION = 3000
const LEAVE = 220

// A pilha de balões, no canto inferior direito. Fica no layout, uma vez só.
export function Toaster() {
  const [items, setItems] = useState<ToastItem[]>([])
  useEffect(() => subscribeToasts(setItems), [])
  return createPortal(
    <div aria-live="polite" className="toaster">
      {items.map((item) => <Toast item={item} key={item.id} />)}
    </div>,
    document.body,
  )
}

const ICONS: Record<ToastKind, string> = { success: '✓', error: '!', info: 'i' }

function Toast({ item }: { item: ToastItem }) {
  const [leaving, setLeaving] = useState(false)
  // o tempo para enquanto o mouse está em cima (dá para ler um erro longo)
  const [paused, setPaused] = useState(false)

  useEffect(() => {
    if (paused) return
    const leave = window.setTimeout(() => setLeaving(true), DURATION)
    return () => window.clearTimeout(leave)
  }, [paused])

  useEffect(() => {
    if (!leaving) return
    const hide = window.setTimeout(() => dismiss(item.id), LEAVE)
    return () => window.clearTimeout(hide)
  }, [leaving, item.id])

  return (
    <div
      className={`toast toast--${item.kind}${leaving ? ' toast--leaving' : ''}${paused ? ' toast--paused' : ''}`}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      role={item.kind === 'error' ? 'alert' : 'status'}
    >
      {item.sha1
        ? <img alt="" className="toast__thumb" src={imageFileUrl(item.sha1)} />
        : <span aria-hidden="true" className="toast__icon">{ICONS[item.kind]}</span>}
      <span className="toast__text">{item.text}</span>
      <button aria-label="Fechar" className="toast__close" onClick={() => setLeaving(true)} type="button">✕</button>
      <span className="toast__timer" key={String(paused)} style={{ animationDuration: `${DURATION}ms` }} />
    </div>
  )
}
