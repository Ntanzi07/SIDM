import { useState } from 'react'
import type { LabelStats } from '../../services/api'
import { Donut } from '../Charts/Donut'
import { MISSING_COLOR, PALETTE } from '../Charts/palette'
import { fmt, pct } from '../../utils/stats'
import '../../pages/Search/Search.css'

// Os cartões de contagem da Pesquisa (e do resumo de uma versão).

export type TaskLabels = LabelStats['tasks'][number]

// Uma tarefa do CVAT: para cada label, em quantas imagens ela aparece e qual
// parte de todas as caixas da tarefa é dela.
export function LabelCard({ task, searchImages }: { task: TaskLabels; searchImages: number }) {
  const names = Object.keys(task.labels).sort()
  return (
    <div className="search-card">
      <h2>Tarefa {task.task}</h2>
      <p className="profile-name-hint">
        {fmt(task.images)} de {fmt(searchImages)} imagens da pesquisa foram anotadas nesta tarefa ({fmt(task.boxes)} caixas). Por label: em quantas dessas imagens ela aparece, e quanto das caixas é dela.
      </p>
      <ul className="search-labels">
        {names.map((name) => {
          const item = task.labels[name]
          return (
            <li key={name}>
              <strong>{name}</strong>
              <div className="search-labels__line">
                <span>Em {fmt(item.images)} de {fmt(task.images)} imagens</span>
                <span className="search-bar__track"><span className="search-bar__fill" style={{ width: pct(item.images, task.images) }} /></span>
                <small>{pct(item.images, task.images)}</small>
              </div>
              <div className="search-labels__line">
                <span>{fmt(item.boxes)} de {fmt(task.boxes)} caixas da tarefa</span>
                <span className="search-bar__track"><span className="search-bar__fill search-bar__fill--boxes" style={{ width: pct(item.boxes, task.boxes) }} /></span>
                <small>{pct(item.boxes, task.boxes)}</small>
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

// Uma escolha de vários: "Todos" ou os itens marcados.
export function Chips({ title, items, chosen, all, allLabel, empty, onAll, onToggle }: {
  title: string
  items: string[]
  chosen: string[]
  all: boolean
  allLabel: string
  empty: string
  onAll: () => void
  onToggle: (item: string) => void
}) {
  return (
    <div className="search-chips">
      <span>{title}</span>
      {items.length === 0 ? (
        <p className="profile-name-hint">{empty}</p>
      ) : (
        <div className="search-chips__list">
          <button className={all ? 'search-chip search-chip--on' : 'search-chip'} onClick={onAll} type="button">{allLabel}</button>
          {items.map((item) => (
            <button
              className={!all && chosen.includes(item) ? 'search-chip search-chip--on' : 'search-chip'}
              key={item}
              onClick={() => onToggle(item)}
              type="button"
            >
              {item}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// Um cartão de contagens: uma barra por valor (a % é do total do cartão) e o
// "Não informado" no fim.
export function CountCard({ title, rows, of, missing, note, active, onPick, donut = false, unit = 'imagens', headline }: {
  title: string
  rows: Array<{ value: string; label: string; count: number }>
  of: number
  missing?: number
  note?: string
  active?: string
  onPick?: (value: string) => void
  // em rosca (com a legenda ao lado). Só para valores que se excluem (cada
  // imagem conta uma vez): numa lista de vários, a soma passa do total.
  donut?: boolean
  unit?: string // o que se conta (o meio da rosca): imagens, objetos
  headline?: { value: number; caption: string } // o número grande em cima das barras
}) {
  const all = missing !== undefined ? [...rows, { value: '', label: 'Não informado', count: missing }] : rows
  if (donut && all.length > 0 && all.length <= 9) {
    return <DonutCard active={active} all={all} note={note} of={of} onPick={onPick} title={title} unit={unit} />
  }
  return (
    <div className="search-card">
      <h2>{title}</h2>
      {headline && <p className="search-card__headline"><strong>{fmt(headline.value)}</strong> <span>{headline.caption}</span></p>}
      {note && <p className="profile-name-hint">{note}</p>}
      {rows.length === 0 && missing === undefined && <p className="profile-name-hint">Nenhuma imagem.</p>}
      <ul className="search-bars">
        {all.map((row) => {
          const clickable = Boolean(onPick && row.value)
          const className = [
            'search-bar',
            clickable ? 'search-bar--clickable' : '',
            active && row.value === active ? 'search-bar--active' : '',
            row.value === '' ? 'search-bar--missing' : '',
          ].filter(Boolean).join(' ')
          const content = (
            <>
              <span className="search-bar__label">{row.label}</span>
              <span className="search-bar__track"><span className="search-bar__fill" style={{ width: pct(row.count, of) }} /></span>
              <span className="search-bar__count">{fmt(row.count)} <small>{pct(row.count, of)}</small></span>
            </>
          )
          return (
            <li key={row.value || '__missing'}>
              {clickable ? (
                <button className={className} onClick={() => onPick?.(row.value)} title={active === row.value ? 'Tirar o filtro' : 'Filtrar por este valor'} type="button">
                  {content}
                </button>
              ) : (
                <div className={className}>{content}</div>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function DonutCard({ title, all, of, note, active, onPick, unit }: {
  title: string
  unit: string
  all: Array<{ value: string; label: string; count: number }>
  of: number
  note?: string
  active?: string
  onPick?: (value: string) => void
}) {
  const [hovered, setHovered] = useState<string | undefined>()
  const segments = all.map((row, i) => ({
    key: row.value || '__missing',
    label: row.label,
    count: row.count,
    color: row.value === '' ? MISSING_COLOR : PALETTE[i % PALETTE.length],
  }))
  const pick = (key: string) => { if (onPick && key !== '__missing') onPick(key) }
  return (
    <div className="search-card">
      <h2>{title}</h2>
      {note && <p className="profile-name-hint">{note}</p>}
      <div className="donut-card">
        <Donut centerLabel={unit} hovered={hovered} onHover={setHovered} onPick={onPick ? pick : undefined} segments={segments} total={of} />
        <ul className="donut-legend">
          {segments.map((item) => {
            const className = [
              'donut-legend__row',
              hovered === item.key ? 'donut-legend__row--on' : '',
              hovered && hovered !== item.key ? 'donut-legend__row--dim' : '',
              active && item.key === active ? 'donut-legend__row--active' : '',
            ].filter(Boolean).join(' ')
            const content = (
              <>
                <i style={{ background: item.color }} />
                <span>{item.label}</span>
                <strong>{fmt(item.count)}</strong>
                <small>{pct(item.count, of)}</small>
              </>
            )
            return (
              <li key={item.key} onMouseEnter={() => setHovered(item.key)} onMouseLeave={() => setHovered(undefined)}>
                {onPick && item.key !== '__missing' ? (
                  <button className={className} onClick={() => pick(item.key)} title={active === item.key ? 'Tirar o filtro' : 'Filtrar por este valor'} type="button">{content}</button>
                ) : (
                  <div className={className}>{content}</div>
                )}
              </li>
            )
          })}
        </ul>
      </div>
    </div>
  )
}
