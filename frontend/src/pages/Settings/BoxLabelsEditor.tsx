import { useEffect, useId, useState } from 'react'
import { listTasks, type Problem } from '../../services/api'
import { HelpTip } from '../../components/HelpTip/HelpTip'
import type { BoxLabel, ObjectType, ProfileField } from '../../types/annotation'

// "Caixas que são este tipo": o nome de uma caixa (ex.: motorcycle), o que
// ela já diz do objeto (tipo = moto) e quais atributos dela são um campo do
// objeto (color → Cor). Na anotação, o Veículo 1 se liga a uma caixa
// motorcycle e já vem com o tipo moto e a cor da caixa.
//
// Como os campos (ProfileEditor.tsx): cada caixa é uma linha com resumo que
// abre para editar; as classes pe-* estão em ProfileEditor.css.

type Props = {
  object: ObjectType
  problems: Problem[] // os de objects[i].box_labels
  readOnly: boolean
  onChange: (labels: BoxLabel[]) => void
}

// As tarefas e as labels de cada uma (uma vez por tela). Uma tarefa pode ter
// muitas labels (o alfabeto de um LPR): escolhe a tarefa, depois a label.
type TaskLabels = { name: string; labels: string[] }
let cache: Promise<TaskLabels[]> | null = null
function taskLabels(): Promise<TaskLabels[]> {
  cache ??= listTasks()
    .then((tasks) => tasks
      .map((task) => ({ name: task.name, labels: [...(task.labels ?? [])].sort() }))
      .filter((task) => task.labels.length > 0)
      .sort((a, b) => a.name.localeCompare(b.name)))
    .catch(() => {
      cache = null
      return []
    })
  return cache
}

// Só os campos que uma label diz bem (o tipo, a categoria), nunca a placa.
const sayable = (field: ProfileField) => field.type === 'choice' || field.type === 'bool'

const nameOf = (field: ProfileField) => field.label || field.key

// "Moto" para "motorcycle"; "Sim"/"Não" no sim/não.
function valueName(field: ProfileField | undefined, value: unknown): string {
  if (typeof value === 'boolean') return value ? 'Sim' : 'Não'
  const text = String(value)
  return field?.options?.find((o) => o.value === text)?.label || text
}

// O resumo de uma caixa na linha fechada.
function summary(label: BoxLabel, fields: ProfileField[]): string {
  const says = Object.entries(label.fields ?? {}).map(([key, value]) => {
    const field = fields.find((f) => f.key === key)
    return `${field ? nameOf(field) : key}: ${valueName(field, value)}`
  })
  const attrs = Object.keys(label.attributes ?? {}).length
  if (attrs) says.push(attrs === 1 ? '1 atributo' : `${attrs} atributos`)
  return says.length ? says.join(', ') : 'não diz nada do objeto ainda'
}

function ValueSelect({ field, value, disabled, onChange, id }: { field: ProfileField; value: unknown; disabled: boolean; onChange: (v: unknown) => void; id: string }) {
  const options = field.type === 'bool' ? [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }] : field.options ?? []
  return (
    <select
      disabled={disabled}
      id={id}
      onChange={(e) => onChange(e.target.value === '' ? undefined : field.type === 'bool' ? e.target.value === 'true' : e.target.value)}
      value={value === undefined || value === null ? '' : String(value)}
    >
      <option value="">Não diz</option>
      {options.map((o) => <option key={o.value} value={o.value}>{o.label || o.value}</option>)}
    </select>
  )
}

export function BoxLabelsEditor({ object, problems, readOnly, onChange }: Props) {
  const uid = useId()
  const [tasks, setTasks] = useState<TaskLabels[]>([])
  // a tarefa escolhida em cada linha (só ajuda a achar a label: o perfil
  // guarda a label, que vale em qualquer tarefa que a tenha)
  const [picked, setPicked] = useState<Record<number, string>>({})
  const [opened, setOpened] = useState<Set<number>>(() => new Set())
  useEffect(() => {
    let alive = true
    void taskLabels().then((list) => { if (alive) setTasks(list) })
    return () => { alive = false }
  }, [])

  const labels = object.box_labels ?? []
  const fields = object.fields.filter(sayable)
  const objectName = (object.label || 'objeto').toLowerCase()
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

  const errorsOf = (j: number) => problems.filter((p) => new RegExp(`box_labels\\[${j}\\]`).test(p.path))
  const loose = problems.filter((p) => !/box_labels\[\d+\]/.test(p.path))
  const isOpen = (j: number) => opened.has(j) || errorsOf(j).length > 0
  const toggle = (j: number) => setOpened((s) => { const n = new Set(s); if (n.has(j)) n.delete(j); else n.add(j); return n })
  const add = () => {
    onChange([...labels, { label: '' }])
    setOpened((s) => new Set(s).add(labels.length))
  }
  const remove = (j: number) => {
    onChange(labels.filter((_, k) => k !== j))
    setOpened((s) => new Set([...s].filter((x) => x !== j).map((x) => (x > j ? x - 1 : x))))
    setPicked((p) => Object.fromEntries(Object.entries(p).filter(([k]) => Number(k) !== j).map(([k, v]) => [Number(k) > j ? Number(k) - 1 : Number(k), v])))
  }

  return (
    <div className="pe-boxes">
      <div className="pe-boxes__head">
        <h5 className="pe-label-row">
          Caixas que são um {objectName}
          <HelpTip label={`Caixas que são um ${objectName}`}>
            Na anotação, cada {objectName} se liga a uma caixa da imagem. O que a caixa diz (motorcycle = moto) já vem preenchido e travado.
          </HelpTip>
        </h5>
      </div>

      {labels.length === 0 && (
        <div className="pe-empty">
          <span>Nenhuma caixa ligada. Sem caixas, quem anota preenche tudo do {objectName} à mão.</span>
        </div>
      )}

      {labels.length > 0 && (
        <ul className="pe-cards">
          {labels.map((label, j) => {
            const id = `${uid}-${j}`
            const open = isOpen(j)
            const errors = errorsOf(j)
            const locked = readOnly || Boolean(label.from)
            const taken = new Set(labels.filter((_, k) => k !== j).map((l) => l.label))
            // a tarefa da linha: a escolhida, ou a primeira que tem a label
            const task = picked[j] ?? tasks.find((t) => t.labels.includes(label.label))?.name ?? ''
            const inTask = tasks.find((t) => t.name === task)?.labels ?? []
            const choices = inTask.filter((l) => !taken.has(l))
            const elsewhere = label.label !== '' && !tasks.some((t) => t.labels.includes(label.label))
            const rows = Object.entries(label.attributes ?? {})
            return (
              <li className={['pe-card', open ? 'pe-card--open' : '', errors.length ? 'pe-card--error' : ''].join(' ')} key={j}>
                <div className="pe-card__bar">
                  <button aria-controls={`${id}-body`} aria-expanded={open} className="pe-card__toggle" onClick={() => toggle(j)} type="button">
                    <span aria-hidden="true" className="pe-chevron" />
                    <span className="pe-card__name pe-mono-name">{label.label || <em>Caixa sem nome</em>}</span>
                    <span className="pe-card__meta">{summary(label, object.fields)}</span>
                    {label.from && <span className="pe-tag">de {label.from}</span>}
                    {errors.length > 0 && <span className="pe-card__flag">{errors.length === 1 ? '1 problema' : `${errors.length} problemas`}</span>}
                  </button>
                </div>

                {open && (
                  <div className="pe-box" id={`${id}-body`}>
                    <div className="pe-grid">
                      <div className="pe-field">
                        <span className="pe-label-row">
                          <label htmlFor={`${id}-task`}>Tarefa</label>
                          <HelpTip label="Tarefa">Só para achar o nome da caixa; ele vale em qualquer tarefa que o tenha.</HelpTip>
                        </span>
                        <select
                          disabled={locked}
                          id={`${id}-task`}
                          onChange={(e) => {
                            setPicked({ ...picked, [j]: e.target.value })
                            // a label escolhida não está na tarefa nova: limpa
                            if (!tasks.find((t) => t.name === e.target.value)?.labels.includes(label.label)) set(j, { ...label, label: '' })
                          }}
                          value={task}
                        >
                          <option value="">Escolha a tarefa</option>
                          {tasks.map((t) => <option key={t.name} value={t.name}>{t.name} ({t.labels.length === 1 ? '1 caixa' : `${t.labels.length} caixas`})</option>)}
                        </select>
                      </div>
                      <div className="pe-field">
                        <label htmlFor={`${id}-label`}>Nome da caixa</label>
                        <select className="pe-mono" disabled={locked || (!task && !elsewhere)} id={`${id}-label`} onChange={(e) => set(j, { ...label, label: e.target.value })} value={label.label}>
                          <option value="">{task ? 'Escolha a caixa' : 'Escolha a tarefa antes'}</option>
                          {elsewhere && <option value={label.label}>{label.label} (não está em nenhuma tarefa)</option>}
                          {label.label && !elsewhere && !choices.includes(label.label) && <option value={label.label}>{label.label}</option>}
                          {choices.map((l) => <option key={l} value={l}>{l}</option>)}
                        </select>
                      </div>
                    </div>

                    {fields.length > 0 && (
                      <fieldset className="pe-says">
                        <legend>O que essa caixa já diz do {objectName}</legend>
                        <div className="pe-grid pe-grid--3">
                          {fields.map((field) => (
                            <div className="pe-field" key={field.key}>
                              <label htmlFor={`${id}-f-${field.key}`}>{nameOf(field)}</label>
                              <ValueSelect disabled={locked} field={field} id={`${id}-f-${field.key}`} onChange={(v) => setValue(j, field.key, v)} value={label.fields?.[field.key]} />
                            </div>
                          ))}
                        </div>
                      </fieldset>
                    )}

                    <div className="pe-attrs" role="group" aria-labelledby={`${id}-attrs`}>
                      <span className="pe-label-row">
                        <span className="pe-attrs__title" id={`${id}-attrs`}>Atributos da caixa que preenchem um campo</span>
                        <HelpTip label="Atributos da caixa">O campo vem do atributo ao ligar a caixa; depois vale o que for editado no SIDM.</HelpTip>
                      </span>
                      {rows.length > 0 && (
                        <div className="pe-attrs__head" aria-hidden="true">
                          <span>Atributo da caixa</span>
                          <span>Campo do {objectName}</span>
                          <span />
                        </div>
                      )}
                      {rows.map(([attr, key], r) => (
                        <div className="pe-attr" key={r}>
                          <input
                            aria-label={`Atributo da caixa ${r + 1}`}
                            className="pe-mono"
                            disabled={locked}
                            onChange={(e) => setAttributes(j, rows.map((row, k) => (k === r ? [e.target.value, row[1]] : row)))}
                            placeholder="color"
                            value={attr}
                          />
                          <select
                            aria-label={`Campo preenchido pelo atributo ${attr || r + 1}`}
                            disabled={locked}
                            onChange={(e) => setAttributes(j, rows.map((row, k) => (k === r ? [row[0], e.target.value] : row)))}
                            value={key}
                          >
                            <option value="">Escolha o campo</option>
                            {object.fields
                              .filter((f) => f.key === key || (label.fields?.[f.key] === undefined && !rows.some((row) => row[1] === f.key)))
                              .map((f) => <option key={f.key} value={f.key}>{nameOf(f)}</option>)}
                          </select>
                          {!locked ? (
                            <button aria-label={`Remover o atributo ${attr || r + 1}`} className="pe-icon pe-icon--danger" onClick={() => setAttributes(j, rows.filter((_, k) => k !== r))} title="Remover atributo" type="button">✕</button>
                          ) : <span />}
                        </div>
                      ))}
                      {!locked && <button className="pe-add" onClick={() => setAttributes(j, [...rows, ['', '']])} type="button">Adicionar atributo</button>}
                    </div>

                    {errors.map((p) => <small className="pe-error" key={p.path + p.msg}>{p.msg}</small>)}
                    {!locked && (
                      <div className="pe-key">
                        <span />
                        <button className="pe-remove" onClick={() => remove(j)} type="button">Remover caixa</button>
                      </div>
                    )}
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}

      {loose.map((p) => <small className="pe-error" key={p.path + p.msg}>{p.msg}</small>)}
      {!readOnly && <button className="pe-add" onClick={add} type="button">Ligar uma caixa</button>}
    </div>
  )
}
