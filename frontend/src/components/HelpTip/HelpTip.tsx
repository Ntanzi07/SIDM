import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import './HelpTip.css'

// O ícone de ajuda (?): a explicação fica fora da tela até a pessoa pedir.
// Abre ao passar o mouse ou focar com Tab; clicar fixa aberto (vale no
// toque); Esc ou clicar fora fecha. O balão vai num portal com posição fixa,
// para não ser cortado por um painel com overflow: hidden.
//
// label: do que é a ajuda ("Separador"), para o leitor de tela.

const GAP = 6
const MARGIN = 8

export function HelpTip({ children, label }: { children: ReactNode; label?: string }) {
  const id = useId()
  const button = useRef<HTMLButtonElement>(null)
  const bubble = useRef<HTMLDivElement>(null)
  const [hover, setHover] = useState(false)
  const [pinned, setPinned] = useState(false)
  const [place, setPlace] = useState<{ top: number; left: number } | null>(null)
  const open = hover || pinned

  const close = useCallback(() => { setHover(false); setPinned(false) }, [])

  // Embaixo do ícone; sem espaço embaixo, em cima. Nunca sai da janela.
  useLayoutEffect(() => {
    if (!open || !button.current || !bubble.current) {
      setPlace(null)
      return
    }
    const anchor = button.current.getBoundingClientRect()
    const box = bubble.current.getBoundingClientRect()
    let top = anchor.bottom + GAP
    if (top + box.height > window.innerHeight - MARGIN) top = Math.max(MARGIN, anchor.top - GAP - box.height)
    const left = Math.min(Math.max(MARGIN, anchor.left + anchor.width / 2 - box.width / 2), window.innerWidth - box.width - MARGIN)
    setPlace({ top, left })
  }, [open])

  useEffect(() => {
    if (!open) return
    // Esc fecha só o balão: na fase de captura, antes do modal em volta (que
    // também fecha com Esc) ficar sabendo.
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.stopPropagation()
      close()
    }
    const onDown = (event: PointerEvent) => {
      const target = event.target as Node
      if (!button.current?.contains(target) && !bubble.current?.contains(target)) close()
    }
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('pointerdown', onDown)
    window.addEventListener('scroll', close, true) // a posição é fixa: rolou, fecha
    window.addEventListener('resize', close)
    return () => {
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('pointerdown', onDown)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
    }
  }, [open, close])

  return (
    <>
      <button
        aria-describedby={open ? id : undefined}
        aria-expanded={open}
        aria-label={label ? `Ajuda: ${label}` : 'Ajuda'}
        className="help-tip"
        onBlur={() => { if (!pinned) setHover(false) }}
        onClick={(event) => { event.preventDefault(); event.stopPropagation(); setPinned((p) => !p) }}
        onFocus={() => setHover(true)}
        onMouseEnter={() => setHover(true)}
        onMouseLeave={() => setHover(false)}
        ref={button}
        type="button"
      >
        ?
      </button>
      {open && createPortal(
        <div
          className="help-tip__bubble"
          id={id}
          onMouseEnter={() => setHover(true)}
          onMouseLeave={() => setHover(false)}
          ref={bubble}
          role="tooltip"
          style={place ?? { top: 0, left: 0, visibility: 'hidden' }}
        >
          {children}
        </div>,
        document.body,
      )}
    </>
  )
}
