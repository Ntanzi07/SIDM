import { useState } from 'react'
import type { ReactNode } from 'react'
import type { LabelStats } from '../../services/api'
import { colorOf } from '../../utils/boxes'
import { useTaskColors } from '../../utils/taskColors'
import { fmt, pct } from '../../utils/stats'
import './Stats.css'

// As peças dos números do Dashboard (a pesquisa de agora e o resumo de uma
// versão): os números de destaque, as caixas por label e os campos em barras.

export type TaskLabels = LabelStats['tasks'][number]

// ---------------------------------------------------------------- destaques

export type Kpi = { label: string; value: number | null; note?: ReactNode }

// Os números de destaque no topo (no máximo quatro). null = ainda contando.
export function KpiStrip({ items }: { items: Kpi[] }) {
  return (
    <dl className="kpi-strip">
      {items.map((item) => (
        <div className="kpi" key={item.label}>
          <dt>{item.label}</dt>
          <dd>
            <strong>{item.value === null ? '…' : fmt(item.value)}</strong>
            {item.note && <span>{item.note}</span>}
          </dd>
        </div>
      ))}
    </dl>
  )
}

// ---------------------------------------------------------------- caixas por label

// Uma tarefa: as labels da que tem mais caixas para a que tem menos, cada uma
// na cor que tem nas caixas da anotação. As sem caixas ficam recolhidas.
export function LabelPanel({ task, searchImages }: { task: TaskLabels; searchImages: number }) {
  useTaskColors() // redesenha quando as cores escolhidas em Tarefas chegam
  const [showEmpty, setShowEmpty] = useState(false)
  const names = Object.keys(task.labels)
  const used = names.filter((name) => task.labels[name].boxes > 0).sort((a, b) => task.labels[b].boxes - task.labels[a].boxes)
  const empty = names.filter((name) => task.labels[name].boxes === 0).sort()
  const top = used.length ? task.labels[used[0]].boxes : 0
  const shown = showEmpty ? [...used, ...empty] : used

  return (
    <section aria-labelledby={`lp-${task.task}`} className="label-panel">
      <header className="label-panel__head">
        <h3 id={`lp-${task.task}`}>{task.task}</h3>
        <p>
          <strong>{fmt(task.boxes)}</strong> caixas em <strong>{fmt(task.images)}</strong> de {fmt(searchImages)} imagens
        </p>
      </header>
      {task.boxes === 0 && <p className="label-panel__none">Nenhuma imagem desta pesquisa tem caixa nesta tarefa.</p>}
      <ul className="label-rows">
        {shown.map((name) => {
          const item = task.labels[name]
          const color = colorOf(name, task.task)
          return (
            <li className={item.boxes === 0 ? 'label-row label-row--empty' : 'label-row'} key={name}>
              <span className="label-row__name">
                <i aria-hidden="true" style={{ background: color }} />
                {name}
              </span>
              <span className="label-row__boxes">{fmt(item.boxes)}</span>
              {/* a barra é relativa à label com mais caixas: a maior enche */}
              <span aria-hidden="true" className="label-row__track">
                <span style={{ width: top ? `${(item.boxes / top) * 100}%` : '0%', background: color }} />
              </span>
              <span className="label-row__images">
                em {fmt(item.images)} {item.images === 1 ? 'imagem' : 'imagens'} <small>{pct(item.images, task.images)}</small>
              </span>
            </li>
          )
        })}
      </ul>
      {empty.length > 0 && (
        <button className="stats-more" onClick={() => setShowEmpty(!showEmpty)} type="button">
          {showEmpty ? 'Esconder as labels sem caixas' : `+${empty.length} ${empty.length === 1 ? 'label sem caixas' : 'labels sem caixas'}`}
        </button>
      )}
    </section>
  )
}

// ---------------------------------------------------------------- campos em barras

type Row = { value: string; label: string; count: number }

// Um campo: os valores da maior contagem para a menor (só os que aparecem; os
// zerados ficam recolhidos), o "Não informado" apagado no fim. Com um valor
// só, vira um cartão compacto. Clicar num valor filtra a pesquisa por ele.
export function BarCard({ title, rows, of, missing = 0, note, active, onPick, unit = 'imagens' }: {
  title: string
  rows: Row[]
  of: number // o total do cartão (a % é dele)
  missing?: number
  note?: string
  active?: string
  onPick?: (value: string) => void
  unit?: string
}) {
  const [showAll, setShowAll] = useState(false)
  const used = rows.filter((row) => row.count > 0).sort((a, b) => b.count - a.count)
  const zero = rows.filter((row) => row.count === 0)
  const top = Math.max(...used.map((row) => row.count), missing, 1)

  // um valor só e nada faltando: a resposta cabe numa linha
  if (used.length === 1 && missing === 0 && !active) {
    const only = used[0]
    return (
      <div className="bar-card bar-card--single">
        <h3>{title}</h3>
        <p className="bar-card__single">
          <Pick active={active} onPick={onPick} row={only}><strong>{only.label}</strong></Pick>
          <span>em todas as {fmt(only.count)} {unit}</span>
        </p>
        {zero.length > 0 && <p className="bar-card__note">{zero.length === 1 ? 'A outra opção não aparece.' : `Nenhuma das outras ${zero.length} opções aparece.`}</p>}
      </div>
    )
  }

  const shown = showAll ? [...used, ...zero] : used
  return (
    <div className="bar-card">
      <h3>{title}</h3>
      {note && <p className="bar-card__note">{note}</p>}
      {used.length === 0 && missing === 0 && <p className="bar-card__note">Nenhum{unit === 'imagens' ? 'a imagem' : ' objeto'} com valor neste campo.</p>}
      <ul className="bars">
        {shown.map((row) => (
          <li key={row.value}>
            <Pick active={active} onPick={onPick} row={row}>
              <span className="bars__label">{row.label}</span>
              <span aria-hidden="true" className="bars__track"><span style={{ width: `${(row.count / top) * 100}%` }} /></span>
              <span className="bars__count">{fmt(row.count)} <small>{pct(row.count, of)}</small></span>
            </Pick>
          </li>
        ))}
        {missing > 0 && (
          <li>
            <div className="bars__row bars__row--missing">
              <span className="bars__label">Não informado</span>
              <span aria-hidden="true" className="bars__track"><span style={{ width: `${(missing / top) * 100}%` }} /></span>
              <span className="bars__count">{fmt(missing)} <small>{pct(missing, of)}</small></span>
            </div>
          </li>
        )}
      </ul>
      {zero.length > 0 && (
        <button className="stats-more" onClick={() => setShowAll(!showAll)} type="button">
          {showAll ? 'Esconder as sem ' + unit : `+${zero.length} sem ${unit}`}
        </button>
      )}
    </div>
  )
}

// Uma linha que filtra ao clicar (ou só texto, sem onPick).
function Pick({ row, active, onPick, children }: { row: Row; active?: string; onPick?: (value: string) => void; children: ReactNode }) {
  const on = active === row.value
  if (!onPick) return <div className="bars__row">{children}</div>
  return (
    <button
      aria-pressed={on}
      className={on ? 'bars__row bars__row--pick bars__row--on' : 'bars__row bars__row--pick'}
      onClick={() => onPick(row.value)}
      title={on ? 'Tirar o filtro' : 'Filtrar por este valor'}
      type="button"
    >
      {children}
    </button>
  )
}
