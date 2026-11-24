import { useEffect, useState } from 'react'
import { fmt, pctText } from './palette'
import './Charts.css'

export type Segment = { key: string; label: string; count: number; color: string }

const SIZE = 148
const R = 56
const STROKE = 18
const C = 2 * Math.PI * R

// Uma rosca: cada fatia cresce ao aparecer; passar o mouse destaca a fatia e
// mostra o valor no meio. hovered/onHover deixam a legenda ao lado
// destacar a mesma fatia.
export function Donut({ segments, total, hovered, onHover, onPick, centerLabel }: {
  segments: Segment[]
  total: number
  hovered?: string
  onHover?: (key: string | undefined) => void
  onPick?: (key: string) => void
  centerLabel: string
}) {
  const [grown, setGrown] = useState(false)
  useEffect(() => {
    const id = requestAnimationFrame(() => setGrown(true))
    return () => cancelAnimationFrame(id)
  }, [])

  const sum = segments.reduce((acc, item) => acc + item.count, 0) || 1
  // onde cada fatia começa (a soma das anteriores)
  const starts = segments.map((_, i) => segments.slice(0, i).reduce((acc, item) => acc + (grown ? (item.count / sum) * C : 0), 0))
  const focus = segments.find((item) => item.key === hovered)

  return (
    <svg className="donut" height={SIZE} role="img" viewBox={`0 0 ${SIZE} ${SIZE}`} width={SIZE}>
      <circle className="donut__track" cx={SIZE / 2} cy={SIZE / 2} fill="none" r={R} strokeWidth={STROKE} />
      <g transform={`rotate(-90 ${SIZE / 2} ${SIZE / 2})`}>
        {segments.map((item, i) => {
          const length = grown ? (item.count / sum) * C : 0
          const gap = item.count > 0 && segments.filter((s) => s.count > 0).length > 1 ? 2 : 0
          const dash = Math.max(0, length - gap)
          const circle = (
            <circle
              className={['donut__slice', hovered && hovered !== item.key ? 'donut__slice--dim' : '', hovered === item.key ? 'donut__slice--on' : '', onPick ? 'donut__slice--clickable' : ''].filter(Boolean).join(' ')}
              cx={SIZE / 2}
              cy={SIZE / 2}
              fill="none"
              key={item.key}
              onClick={() => onPick?.(item.key)}
              onMouseEnter={() => onHover?.(item.key)}
              onMouseLeave={() => onHover?.(undefined)}
              r={R}
              stroke={item.color}
              strokeDasharray={`${dash} ${C - dash}`}
              strokeDashoffset={-starts[i]}
              strokeWidth={STROKE}
            >
              <title>{`${item.label}: ${fmt(item.count)} (${pctText(item.count, total)})`}</title>
            </circle>
          )
          return circle
        })}
      </g>
      <text className="donut__big" textAnchor="middle" x={SIZE / 2} y={SIZE / 2 + 2}>
        {focus ? pctText(focus.count, total) : fmt(total)}
      </text>
      <text className="donut__small" textAnchor="middle" x={SIZE / 2} y={SIZE / 2 + 20}>
        {focus ? (focus.label.length > 16 ? `${focus.label.slice(0, 15)}…` : focus.label) : centerLabel}
      </text>
    </svg>
  )
}
