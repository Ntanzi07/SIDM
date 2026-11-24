import type { Problem } from '../../services/api'
import type { BoxLabel, ObjectType, ProfileField } from '../../types/annotation'

// "Caixas que são este tipo": a label de uma caixa (ex.: motorcycle) e o que
// ela já diz do objeto (tipo = moto). Na anotação, o Veículo 1 pode ser
// ligado a uma caixa motorcycle e já vem com o tipo moto.

type Props = {
  object: ObjectType
  problems: Problem[] // os de objects[i].box_labels
  readOnly: boolean
  onChange: (labels: BoxLabel[]) => void
}

// Só os campos de um valor fácil de dizer pela label.
const usable = (field: ProfileField) => field.type === 'choice' || field.type === 'bool' || field.type === 'text' || field.type === 'number'

function ValueInput({ field, value, disabled, onChange }: { field: ProfileField; value: unknown; disabled: boolean; onChange: (v: unknown) => void }) {
  const text = value === undefined || value === null ? '' : String(value)
  if (field.type === 'choice' || field.type === 'bool') {
    const options = field.type === 'bool' ? [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }] : field.options ?? []
    return (
      <select
        disabled={disabled}
        onChange={(e) => onChange(e.target.value === '' ? undefined : field.type === 'bool' ? e.target.value === 'true' : e.target.value)}
        value={text}
      >
        <option value="">—</option>
        {options.map((o) => <option key={o.value} value={o.value}>{o.label || o.value}</option>)}
      </select>
    )
  }
  return (
    <input
      disabled={disabled}
      onChange={(e) => onChange(e.target.value === '' ? undefined : field.type === 'number' ? Number(e.target.value) : e.target.value)}
      placeholder="—"
      type={field.type === 'number' ? 'number' : 'text'}
      value={text}
    />
  )
}

export function BoxLabelsEditor({ object, problems, readOnly, onChange }: Props) {
  const labels = object.box_labels ?? []
  const fields = object.fields.filter(usable)
  const set = (j: number, label: BoxLabel) => onChange(labels.map((l, k) => (k === j ? label : l)))
  const setValue = (j: number, key: string, value: unknown) => {
    const values = { ...(labels[j].fields ?? {}) }
    if (value === undefined) delete values[key]
    else values[key] = value
    set(j, { ...labels[j], fields: Object.keys(values).length ? values : undefined })
  }

  return (
    <div className="ap-box-labels">
      <strong>Caixas que são um(a) {object.label || object.key}</strong>
      <p className="ap-small">
        A label de uma caixa (ex.: motorcycle) que é um objeto deste tipo. Na anotação, o objeto se liga à caixa, e os
        valores abaixo já vêm preenchidos (e travados) por ela.
      </p>
      {labels.map((label, j) => {
        const locked = readOnly || Boolean(label.from)
        return (
          <div className="ap-row ap-box-label" key={j}>
            <label className="ap-inline">
              <span>Label da caixa{label.from ? ` (de ${label.from})` : ''}</span>
              <input disabled={locked} onChange={(e) => set(j, { ...label, label: e.target.value.trim() })} placeholder="ex.: motorcycle" value={label.label} />
            </label>
            {fields.map((field) => (
              <label className="ap-inline" key={field.key}>
                <span>{field.label || field.key}</span>
                <ValueInput disabled={locked} field={field} onChange={(v) => setValue(j, field.key, v)} value={label.fields?.[field.key]} />
              </label>
            ))}
            {!locked && (
              <div className="ap-row-actions">
                <button className="button button--danger button--sm" onClick={() => onChange(labels.filter((_, k) => k !== j))} type="button">Remover</button>
              </div>
            )}
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
