import { useEffect, useState } from 'react'
import { listTasks, type Problem } from '../../services/api'
import type { BoxLabel, ObjectType, ProfileField } from '../../types/annotation'

// "Caixas que são este tipo": a label de uma caixa (ex.: motorcycle), o que
// ela já diz do objeto (tipo = moto) e quais atributos dela (do CVAT) são um
// campo do objeto (color → Cor). Na anotação, o Veículo 1 se liga a uma
// caixa motorcycle e já vem com o tipo moto e a cor da caixa.

type Props = {
  object: ObjectType
  problems: Problem[] // os de objects[i].box_labels
  readOnly: boolean
  onChange: (labels: BoxLabel[]) => void
}

// As labels das tarefas (uma vez por tela).
let cache: Promise<string[]> | null = null
function taskLabels(): Promise<string[]> {
  cache ??= listTasks()
    .then((tasks) => [...new Set(tasks.flatMap((task) => task.labels ?? []))].sort())
    .catch(() => {
      cache = null
      return []
    })
  return cache
}

// Só os campos que uma label diz bem (o tipo, a categoria), nunca a placa.
const sayable = (field: ProfileField) => field.type === 'choice' || field.type === 'bool'

function ValueSelect({ field, value, disabled, onChange }: { field: ProfileField; value: unknown; disabled: boolean; onChange: (v: unknown) => void }) {
  const options = field.type === 'bool' ? [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }] : field.options ?? []
  return (
    <select
      disabled={disabled}
      onChange={(e) => onChange(e.target.value === '' ? undefined : field.type === 'bool' ? e.target.value === 'true' : e.target.value)}
      value={value === undefined || value === null ? '' : String(value)}
    >
      <option value="">—</option>
      {options.map((o) => <option key={o.value} value={o.value}>{o.label || o.value}</option>)}
    </select>
  )
}

export function BoxLabelsEditor({ object, problems, readOnly, onChange }: Props) {
  const [known, setKnown] = useState<string[]>([])
  useEffect(() => {
    let alive = true
    void taskLabels().then((labels) => { if (alive) setKnown(labels) })
    return () => { alive = false }
  }, [])

  const labels = object.box_labels ?? []
  const fields = object.fields.filter(sayable)
  const set = (j: number, label: BoxLabel) => onChange(labels.map((l, k) => (k === j ? label : l)))
  const setValue = (j: number, key: string, value: unknown) => {
    const values = { ...(labels[j].fields ?? {}) }
    if (value === undefined) delete values[key]
    else values[key] = value
    set(j, { ...labels[j], fields: Object.keys(values).length ? values : undefined })
  }
  // atributos: a ordem da tela é a das entradas do mapa
  const setAttributes = (j: number, rows: Array<[string, string]>) =>
    set(j, { ...labels[j], attributes: rows.length ? Object.fromEntries(rows) : undefined })

  return (
    <div className="ap-box-labels">
      <strong>Caixas que são um(a) {object.label || object.key}</strong>
      <p className="ap-small">
        Escolha a label de uma caixa das tarefas (ex.: motorcycle) que é um objeto deste tipo. Na anotação, o objeto se liga
        à caixa: o que a label diz vem preenchido e travado, e os atributos que são um campo preenchem esse campo.
      </p>
      {labels.map((label, j) => {
        const locked = readOnly || Boolean(label.from)
        const taken = new Set(labels.filter((_, k) => k !== j).map((l) => l.label))
        const choices = known.filter((l) => !taken.has(l))
        const rows = Object.entries(label.attributes ?? {})
        return (
          <div className="ap-box-label-card" key={j}>
            <div className="ap-row ap-box-label">
              <label className="ap-inline">
                <span>Label da caixa{label.from ? ` (de ${label.from})` : ''}</span>
                <select disabled={locked} onChange={(e) => set(j, { ...label, label: e.target.value })} value={label.label}>
                  <option value="">Escolha…</option>
                  {label.label && !choices.includes(label.label) && <option value={label.label}>{label.label} (não está em nenhuma tarefa)</option>}
                  {choices.map((l) => <option key={l} value={l}>{l}</option>)}
                </select>
              </label>
              {fields.map((field) => (
                <label className="ap-inline" key={field.key}>
                  <span>{field.label || field.key}</span>
                  <ValueSelect disabled={locked} field={field} onChange={(v) => setValue(j, field.key, v)} value={label.fields?.[field.key]} />
                </label>
              ))}
              {!locked && (
                <div className="ap-row-actions">
                  <button className="button button--danger button--sm" onClick={() => onChange(labels.filter((_, k) => k !== j))} type="button">Remover</button>
                </div>
              )}
            </div>
            <div className="ap-box-attrs">
              <span className="ap-small">Atributos da caixa que são um campo (o campo vem do atributo ao ligar; depois vale o do SIDM):</span>
              {rows.map(([attr, key], r) => (
                <div className="ap-row" key={r}>
                  <label className="ap-inline">
                    <span>Atributo no CVAT</span>
                    <input
                      disabled={locked}
                      onChange={(e) => setAttributes(j, rows.map((row, k) => (k === r ? [e.target.value, row[1]] : row)))}
                      placeholder="ex.: color"
                      value={attr}
                    />
                  </label>
                  <label className="ap-inline">
                    <span>é o campo</span>
                    <select disabled={locked} onChange={(e) => setAttributes(j, rows.map((row, k) => (k === r ? [row[0], e.target.value] : row)))} value={key}>
                      <option value="">Escolha…</option>
                      {object.fields
                        .filter((f) => f.key === key || (label.fields?.[f.key] === undefined && !rows.some((row) => row[1] === f.key)))
                        .map((f) => <option key={f.key} value={f.key}>{f.label || f.key}</option>)}
                    </select>
                  </label>
                  {!locked && (
                    <div className="ap-row-actions">
                      <button className="button button--danger button--sm" onClick={() => setAttributes(j, rows.filter((_, k) => k !== r))} type="button">Remover</button>
                    </div>
                  )}
                </div>
              ))}
              {!locked && (
                <button className="button button--secondary button--sm" onClick={() => setAttributes(j, [...rows, ['', '']])} type="button">+ Atributo</button>
              )}
            </div>
          </div>
        )
      })}
      {problems.map((p) => <small className="ap-error" key={p.path + p.msg}>{p.path.replace(/^objects\[\d+\]\./, '')}: {p.msg}</small>)}
      {!readOnly && (
        <button className="button button--secondary button--sm" onClick={() => onChange([...labels, { label: '' }])} type="button">+ Caixa</button>
      )}
    </div>
  )
}
