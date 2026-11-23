import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import './ZoomView.css'

const MIN = 1
const MAX = 8
const STEP = 1.5 // cada clique no + / − multiplica (ou divide) por isto

type Offset = { x: number; y: number }

// Zoom e arrasto para uma imagem (com as caixas por cima). O conteúdo continua
// no tamanho normal da página; o zoom é um transform (translate + scale), e as
// caixas acompanham sozinhas porque são em porcentagem. --zoom vai para o CSS,
// para as bordas e os nomes das caixas não engrossarem junto.
//
// + / − / clique duplo / Ctrl + roda do mouse: zoom (no ponto do mouse).
// Arrastar: mover a imagem com zoom. "Ajustar": volta ao tamanho inteiro.
export function ZoomView({ children, resetKey }: { children: ReactNode; resetKey?: string }) {
  const box = useRef<HTMLDivElement | null>(null)
  const content = useRef<HTMLDivElement | null>(null)
  const [zoom, setZoom] = useState(1)
  const [offset, setOffset] = useState<Offset>({ x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  // o zoom e o deslocamento atuais para os eventos (roda e arrasto chegam
  // várias vezes entre uma renderização e outra)
  const view = useRef({ zoom, offset })
  useLayoutEffect(() => { view.current = { zoom, offset } }, [zoom, offset])

  const apply = useCallback((z: number, o: Offset) => {
    view.current = { zoom: z, offset: o }
    setZoom(z)
    setOffset(o)
  }, [])
  const moved = useRef(false)

  // outra imagem: volta ao tamanho inteiro
  const [lastKey, setLastKey] = useState(resetKey)
  if (resetKey !== lastKey) {
    setLastKey(resetKey)
    setZoom(1)
    setOffset({ x: 0, y: 0 })
  }

  // o deslocamento não deixa a imagem sair do quadro (nem mostrar o vazio)
  const clamp = useCallback((z: number, o: Offset): Offset => {
    const b = box.current
    const c = content.current
    if (!b || !c) return o
    const maxX = Math.max(0, (c.offsetWidth * z - b.clientWidth) / 2)
    const maxY = Math.max(0, (c.offsetHeight * z - b.clientHeight) / 2)
    return { x: Math.max(-maxX, Math.min(maxX, o.x)), y: Math.max(-maxY, Math.min(maxY, o.y)) }
  }, [])

  // zoom mantendo parado o ponto (px, py) do quadro (o mouse, ou o centro)
  const zoomTo = useCallback((next: number, px?: number, py?: number) => {
    const b = box.current
    const c = content.current
    if (!b || !c) return
    const z = Math.max(MIN, Math.min(MAX, next))
    const { zoom: z0, offset: o } = view.current
    // o centro do conteúdo no quadro, sem o transform
    const cx = c.offsetLeft + c.offsetWidth / 2
    const cy = c.offsetTop + c.offsetHeight / 2
    const sx = px ?? b.clientWidth / 2
    const sy = py ?? b.clientHeight / 2
    // o ponto da imagem embaixo do mouse continua embaixo do mouse
    const qx = (sx - cx - o.x) / z0
    const qy = (sy - cy - o.y) / z0
    apply(z, clamp(z, { x: sx - cx - z * qx, y: sy - cy - z * qy }))
  }, [apply, clamp])

  // Ctrl + roda: zoom no ponto do mouse (sem Ctrl a página rola normal).
  // Precisa de um listener não passivo para impedir o zoom do navegador.
  useEffect(() => {
    const b = box.current
    if (!b) return
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return
      event.preventDefault()
      const rect = b.getBoundingClientRect()
      const factor = Math.exp(-event.deltaY * 0.0025)
      zoomTo(view.current.zoom * factor, event.clientX - rect.left, event.clientY - rect.top)
    }
    b.addEventListener('wheel', onWheel, { passive: false })
    return () => b.removeEventListener('wheel', onWheel)
  }, [zoomTo])

  function startDrag(event: React.PointerEvent) {
    if (view.current.zoom <= 1 || event.button !== 0) return
    const start = { x: event.clientX, y: event.clientY, o: view.current.offset }
    moved.current = false
    setDragging(true)
    const onMove = (e: PointerEvent) => {
      const dx = e.clientX - start.x
      const dy = e.clientY - start.y
      if (Math.abs(dx) + Math.abs(dy) > 3) moved.current = true
      apply(view.current.zoom, clamp(view.current.zoom, { x: start.o.x + dx, y: start.o.y + dy }))
    }
    const onUp = () => {
      setDragging(false)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  function doubleClick(event: React.MouseEvent) {
    const rect = box.current?.getBoundingClientRect()
    if (!rect) return
    if (view.current.zoom >= MAX) zoomTo(1)
    else zoomTo(view.current.zoom * 2, event.clientX - rect.left, event.clientY - rect.top)
  }

  const zoomed = zoom > 1.001
  return (
    <div className="zoom-view">
      <div
        className={['zoom-view__frame', zoomed ? 'zoom-view__frame--zoomed' : '', dragging ? 'zoom-view__frame--dragging' : ''].filter(Boolean).join(' ')}
        onClickCapture={(event) => {
          // fim de um arrasto: não conta como clique numa caixa
          if (moved.current) {
            event.stopPropagation()
            moved.current = false
          }
        }}
        onDoubleClick={doubleClick}
        onPointerDown={startDrag}
        ref={box}
      >
        <div
          className="zoom-view__content"
          ref={content}
          style={{ transform: `translate(${offset.x}px, ${offset.y}px) scale(${zoom})`, ['--zoom' as string]: zoom }}
        >
          {children}
        </div>
      </div>
      <div className="zoom-view__controls" role="group" aria-label="Zoom">
        <button aria-label="Diminuir o zoom" disabled={!zoomed} onClick={() => zoomTo(zoom / STEP)} type="button">−</button>
        <span>{Math.round(zoom * 100)}%</span>
        <button aria-label="Aumentar o zoom" disabled={zoom >= MAX} onClick={() => zoomTo(zoom * STEP)} type="button">+</button>
        <button className="zoom-view__fit" disabled={!zoomed} onClick={() => zoomTo(1)} title="Voltar à imagem inteira" type="button">Ajustar</button>
      </div>
      <p className="zoom-view__hint">Clique duplo ou Ctrl + roda do mouse para o zoom · arraste para mover</p>
    </div>
  )
}
