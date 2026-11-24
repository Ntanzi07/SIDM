import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { fmt, spanText } from './palette'
import './Charts.css'

export type TimelinePoint = {
  number: number // 0 = o agora (não salvo): ponto tracejado, na data de hoje
  date: string // ISO
  message: string
  by: string
  images: number
  // o que mudou desde a anterior (sem: ainda carregando)
  changes?: { added: number; removed: number; changed: number }
}

const TOP = 150 // altura da linha de imagens
const GAP = 26 // eixo de datas
const BARS = 64 // as mudanças embaixo
const PAD = { left: 70, right: 34, top: 14 }

const dateText = (t: number) => new Date(t).toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', year: '2-digit' })

function useWidth(ref: React.RefObject<HTMLDivElement | null>) {
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const node = ref.current
    if (!node) return
    setWidth(node.clientWidth)
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(node)
    return () => observer.disconnect()
  }, [ref])
  return width
}

// A linha do tempo das versões: em cima, quantas imagens cada versão tem, na
// data em que foi criada (o espaço entre os pontos é o tempo que passou);
// embaixo, o que mudou em cada uma (entraram/alteradas para cima, saíram para
// baixo). Passar o mouse mostra o detalhe; clicar escolhe a versão.
export function Timeline({ points, selected, onSelect }: { points: TimelinePoint[]; selected?: number; onSelect?: (number: number) => void }) {
  const box = useRef<HTMLDivElement | null>(null)
  const width = useWidth(box)
  const [hover, setHover] = useState<number | null>(null)
  const [drawn, setDrawn] = useState(false)
  useEffect(() => {
    const id = requestAnimationFrame(() => setDrawn(true))
    return () => cancelAnimationFrame(id)
  }, [])

  const sorted = [...points].sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime())
  const times = sorted.map((item) => new Date(item.date).getTime())
  const t0 = Math.min(...times)
  const t1 = Math.max(...times)
  const span = t1 - t0
  const innerW = Math.max(0, width - PAD.left - PAD.right)
  // uma versão só (ou todas no mesmo instante): no meio
  const xOf = (t: number) => PAD.left + (span > 0 ? ((t - t0) / span) * innerW : innerW / 2)
  const maxImages = Math.max(1, ...sorted.map((item) => item.images))
  const yMax = maxImages * 1.15
  const yOf = (n: number) => PAD.top + (1 - n / yMax) * (TOP - PAD.top)
  const maxChange = Math.max(1, ...sorted.map((item) => Math.max((item.changes?.added ?? 0) + (item.changes?.changed ?? 0), item.changes?.removed ?? 0)))
  const base = TOP + GAP + BARS / 2
  const hOf = (n: number) => (n / maxChange) * (BARS / 2 - 4)
  const barW = Math.max(6, Math.min(16, innerW / Math.max(sorted.length, 1) / 2.5))

  const xs = times.map(xOf)
  // a linha das versões gravadas; até o agora, tracejada
  const saved = sorted.filter((item) => item.number !== 0)
  const savedXs = xs.filter((_, i) => sorted[i].number !== 0)
  const line = saved.map((item, i) => `${i === 0 ? 'M' : 'L'}${savedXs[i].toFixed(1)},${yOf(item.images).toFixed(1)}`).join(' ')
  const nowIndex = sorted.findIndex((item) => item.number === 0)
  const nowLine = nowIndex > 0
    ? `M${xs[nowIndex - 1].toFixed(1)},${yOf(sorted[nowIndex - 1].images).toFixed(1)} L${xs[nowIndex].toFixed(1)},${yOf(sorted[nowIndex].images).toFixed(1)}`
    : ''
  const all = sorted.map((item, i) => `${i === 0 ? 'M' : 'L'}${xs[i].toFixed(1)},${yOf(item.images).toFixed(1)}`).join(' ')
  const area = sorted.length > 0 ? `${all} L${xs[xs.length - 1].toFixed(1)},${TOP} L${xs[0].toFixed(1)},${TOP} Z` : ''

  // as datas no eixo: todas, se couberem; senão umas 6 espalhadas
  const candidates = sorted.length <= Math.max(2, Math.floor(innerW / 90))
    ? times
    : Array.from({ length: 6 }, (_, i) => t0 + (span * i) / 5)
  // datas muito perto uma da outra se sobrepõem: fica a mais nova
  const ticks: number[] = []
  for (const t of candidates) {
    if (ticks.length > 0 && xOf(t) - xOf(ticks[ticks.length - 1]) < 64) ticks[ticks.length - 1] = t
    else ticks.push(t)
  }

  // as linhas de grade do eixo de imagens
  const grid = [0, 0.5, 1].map((f) => Math.round(maxImages * f))

  // o mouse escolhe a versão mais perto no eixo x
  function move(event: React.MouseEvent<SVGSVGElement>) {
    const rect = event.currentTarget.getBoundingClientRect()
    const x = event.clientX - rect.left
    let best = 0
    xs.forEach((value, i) => { if (Math.abs(value - x) < Math.abs(xs[best] - x)) best = i })
    setHover(sorted.length > 0 ? best : null)
  }

  const h = hover !== null ? sorted[hover] : null
  const prev = hover !== null && hover > 0 ? sorted[hover - 1] : null
  const height = TOP + GAP + BARS

  return (
    <div className="timeline" ref={box}>
      {width > 0 && (
        <svg
          className={drawn ? 'timeline__svg timeline__svg--drawn' : 'timeline__svg'}
          height={height}
          onClick={() => { if (h) onSelect?.(h.number) }}
          onMouseLeave={() => setHover(null)}
          onMouseMove={move}
          width={width}
        >
          {grid.map((value) => (
            <g key={value}>
              <line className="timeline__grid" x1={PAD.left} x2={width - PAD.right} y1={yOf(value)} y2={yOf(value)} />
              <text className="timeline__axis" textAnchor="end" x={PAD.left - 8} y={yOf(value) + 4}>{fmt(value)}</text>
            </g>
          ))}
          <path className="timeline__area" d={area} />
          {line && <path className="timeline__line" d={line} pathLength={1} />}
          {nowLine && <path className="timeline__now-line" d={nowLine} />}

          {ticks.map((t, i) => (
            // a do fim encosta na direita (centrada, sairia do quadro)
            <text className="timeline__axis" key={i} textAnchor={xOf(t) > width - PAD.right - 20 ? 'end' : 'middle'} x={Math.min(xOf(t), width - 2)} y={TOP + 18}>
              {nowIndex >= 0 && t === times[nowIndex] ? 'agora' : dateText(t)}
            </text>
          ))}

          <line className="timeline__base" x1={PAD.left} x2={width - PAD.right} y1={base} y2={base} />
          <text className="timeline__axis" textAnchor="end" x={PAD.left - 8} y={base + 4}>mudanças</text>
          {sorted.map((item, i) => {
            const c = item.changes
            if (!c) return null
            const up1 = hOf(c.added)
            const up2 = hOf(c.changed)
            const down = hOf(c.removed)
            const x = xs[i] - barW / 2
            return (
              <g className="timeline__bars" key={item.number} style={{ transitionDelay: `${i * 40}ms` }}>
                {c.added > 0 && <rect className="timeline__bar timeline__bar--added" height={up1} rx="2" width={barW} x={x} y={base - up1} />}
                {c.changed > 0 && <rect className="timeline__bar timeline__bar--changed" height={up2} rx="2" width={barW} x={x} y={base - up1 - up2} />}
                {c.removed > 0 && <rect className="timeline__bar timeline__bar--removed" height={down} rx="2" width={barW} x={x} y={base} />}
              </g>
            )
          })}

          {h && hover !== null && <line className="timeline__guide" x1={xs[hover]} x2={xs[hover]} y1={PAD.top} y2={height} />}
          {sorted.map((item, i) => (
            <circle
              className={[
                'timeline__dot',
                item.number === 0 ? 'timeline__dot--now' : '',
                item.number === selected ? 'timeline__dot--selected' : '',
                hover === i ? 'timeline__dot--hover' : '',
              ].filter(Boolean).join(' ')}
              cx={xs[i]}
              cy={yOf(item.images)}
              key={item.number}
              r={item.number === selected ? 6 : 4.5}
              style={{ transitionDelay: `${300 + i * 40}ms` }}
            />
          ))}
        </svg>
      )}

      {h && hover !== null && (
        <div className={xs[hover] > width - 260 ? 'timeline__tip timeline__tip--left' : 'timeline__tip'} style={{ left: xs[hover], top: yOf(h.images) }}>
          <strong>{h.number === 0 ? 'Agora · ainda não salvo' : `v${h.number}`} · {new Date(h.date).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })}</strong>
          {h.number !== 0 && <span className="timeline__tip-message">{h.message}</span>}
          <span>{fmt(h.images)} imagem(ns){prev && <em className={h.images >= prev.images ? 'up' : 'down'}> {h.images >= prev.images ? '+' : '−'}{fmt(Math.abs(h.images - prev.images))}</em>}</span>
          {h.changes && (
            <span className="timeline__tip-changes">
              <i className="added">+{fmt(h.changes.added)} entraram</i>
              <i className="changed">~{fmt(h.changes.changed)} alteradas</i>
              <i className="removed">−{fmt(h.changes.removed)} saíram</i>
            </span>
          )}
          <small>
            {h.by}
            {prev ? ` · ${spanText(new Date(h.date).getTime() - new Date(prev.date).getTime())} depois da v${prev.number}` : h.number === 0 ? '' : ' · a primeira versão'}
          </small>
        </div>
      )}
    </div>
  )
}
