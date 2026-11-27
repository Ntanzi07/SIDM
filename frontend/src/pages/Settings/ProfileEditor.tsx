import { useId, useRef, useState } from 'react'
import type { KeyboardEvent, ReactNode } from 'react'
import type { Problem } from '../../services/api'
import { HelpTip } from '../../components/HelpTip/HelpTip'
import type { FieldType, FormValue, ObjectType, ProfileField, ProfileOption } from '../../types/annotation'
import { newField, optionValue, pathId, slug, summary, TYPES, under, withType } from './profileEditorUtils'
import { FieldInput } from '../Annotation/AnnotationFields'
import '../Annotation/Annotation.css'
import './ProfileEditor.css'

// As peças do editor de perfil de anotação. Cada campo é uma linha que
// resume o que ele é (nome, tipo, opções, obrigatório) e abre para editar;
// aberto, mostra ao lado como o campo aparece na tela de anotação. Quem
// confere tudo é o backend: os problemas voltam com o caminho
// ("fields[2].options[1].code") e o campo com problema abre sozinho.

// ---------------------------------------------------------------- opções

function OptionsTable({ field, onChange, readOnly, problems, base }: { field: ProfileField; onChange: (options: ProfileOption[]) => void; readOnly: boolean; problems: Problem[]; base: string }) {
  const options = field.options ?? []
  const table = useRef<HTMLDivElement>(null)
  const set = (i: number, patch: Partial<ProfileOption>) => onChange(options.map((o, j) => (j === i ? { ...o, ...patch } : o)))
  const add = () => {
    onChange([...options, { value: '', label: '' }])
    // o foco vai para o nome da opção nova
    requestAnimationFrame(() => table.current?.querySelector<HTMLInputElement>('.pe-option:last-of-type input')?.focus())
  }
  // Enter na última opção cria a próxima (digitar uma lista sem o mouse).
  const onEnter = (i: number) => (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter') return
    event.preventDefault()
    if (i === options.length - 1) add()
    else table.current?.querySelectorAll<HTMLInputElement>('.pe-option')[i + 1]?.querySelector('input')?.focus()
  }

  // multi: uma opção pode ser a exclusiva ("Sem avarias": não convive com as outras)
  const multi = field.type === 'multi'
  const setExclusive = (i: number, on: boolean) => onChange(options.map((o, j) => {
    const exclusive = j === i ? on : on ? false : o.exclusive // só uma por campo
    return exclusive ? { ...o, exclusive } : { ...o, exclusive: undefined }
  }))

  return (
    <div className={multi ? 'pe-options pe-options--multi' : 'pe-options'} ref={table} role="group" aria-label={`Opções de ${field.label || 'campo'}`}>
      <div className="pe-options__head" aria-hidden="true">
        <span>Nome na tela</span>
        <span>Valor guardado</span>
        <span>Sigla (opcional)</span>
        {multi && <span>Exclusiva</span>}
        <span />
      </div>
      {options.length === 0 && <p className="pe-muted">Nenhuma opção ainda. Uma lista precisa de pelo menos uma, como Preto ou Branco.</p>}
      {options.map((option, i) => {
        const errors = under(problems, `${base}.options[${i}]`)
        const n = `opção ${i + 1}`
        return (
          <div className={errors.length ? 'pe-option pe-option--error' : 'pe-option'} key={i}>
            <input
              aria-label={`Nome na tela da ${n}`}
              disabled={readOnly}
              onBlur={() => { if (!option.value && option.label) set(i, { value: optionValue(option.label) }) }}
              onChange={(e) => set(i, { label: e.target.value })}
              onKeyDown={onEnter(i)}
              placeholder={i === 0 && !option.value ? 'Preto' : ''}
              value={option.label ?? ''}
            />
            <input
              aria-label={`Valor guardado da ${n}`}
              className="pe-mono"
              disabled={readOnly}
              onChange={(e) => set(i, { value: e.target.value.toLowerCase() })}
              onKeyDown={onEnter(i)}
              placeholder={option.label ? optionValue(option.label) : i === 0 ? 'preto' : ''}
              value={option.value}
            />
            <input
              aria-label={`Sigla da ${n} (opcional)`}
              className="pe-mono"
              disabled={readOnly}
              maxLength={8}
              onChange={(e) => set(i, { code: e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '') || undefined })}
              onKeyDown={onEnter(i)}
              placeholder={i === 0 && !option.label ? 'PT' : ''}
              value={option.code ?? ''}
            />
            {multi && (
              <label className="pe-option__exclusive">
                <input
                  aria-label={`${option.label || n}: exclusiva (não combina com as outras)`}
                  checked={Boolean(option.exclusive)}
                  disabled={readOnly}
                  onChange={(e) => setExclusive(i, e.target.checked)}
                  type="checkbox"
                />
              </label>
            )}
            {!readOnly ? (
              <button aria-label={`Remover a opção ${option.label || i + 1}`} className="pe-icon pe-icon--danger" onClick={() => onChange(options.filter((_, j) => j !== i))} title="Remover opção" type="button">✕</button>
            ) : <span />}
            {errors.map((p) => <small className="pe-error" key={p.path + p.msg}>{p.msg}</small>)}
          </div>
        )
      })}
      <div className="pe-options__foot">
        {!readOnly && <button className="pe-add" onClick={add} type="button">Adicionar opção</button>}
        <HelpTip label="Valor guardado e sigla">
          <ul>
            <li><strong>Valor guardado:</strong> vai para o banco e o export. Vazio, sai do nome.</li>
            <li><strong>Sigla:</strong> a forma curta no nome do arquivo exportado (Preto vira PT em <code>…_PT_…</code>); o import também aceita.</li>
            <li>Se uma opção tiver sigla, todas precisam ter.</li>
            {multi && <li><strong>Exclusiva:</strong> a opção "nenhum dos outros" (ex.: Sem avarias). Marcar ela tira as outras, e marcar outra tira ela. Só uma por campo.</li>}
          </ul>
        </HelpTip>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- valor inicial

function DefaultEditor({ field, onChange, readOnly, id }: { field: ProfileField; onChange: (value: unknown) => void; readOnly: boolean; id: string }) {
  const value = field.default
  if (field.type === 'ref' || field.type === 'datetime') return null
  if (field.type === 'choice' || field.type === 'bool') {
    const options = field.type === 'bool' ? [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }] : field.options ?? []
    return (
      <div className="pe-field">
        <label htmlFor={id}>Valor inicial</label>
        <select
          disabled={readOnly}
          id={id}
          onChange={(e) => onChange(e.target.value === '' ? undefined : field.type === 'bool' ? e.target.value === 'true' : e.target.value)}
          value={value === undefined || value === null ? '' : String(value)}
        >
          <option value="">Nenhum</option>
          {options.filter((o) => o.value).map((o) => <option key={o.value} value={o.value}>{o.label || o.value}</option>)}
        </select>
      </div>
    )
  }
  if (field.type === 'multi') {
    const chosen = Array.isArray(value) ? value.map(String) : []
    const options = (field.options ?? []).filter((o) => o.value)
    if (options.length === 0) return null
    return (
      <fieldset className="pe-field pe-checks">
        <legend>Já marcadas no início</legend>
        {options.map((o) => (
          <label key={o.value}>
            <input
              checked={chosen.includes(o.value)}
              disabled={readOnly}
              onChange={(e) => {
                const next = e.target.checked ? [...chosen, o.value] : chosen.filter((v) => v !== o.value)
                onChange(next.length ? next : undefined)
              }}
              type="checkbox"
            />
            {o.label || o.value}
          </label>
        ))}
      </fieldset>
    )
  }
  return (
    <div className="pe-field">
      <label htmlFor={id}>Valor inicial</label>
      <input
        disabled={readOnly}
        id={id}
        onChange={(e) => {
          const text = e.target.value
          onChange(text === '' ? undefined : field.type === 'number' ? Number(text) : text)
        }}
        type={field.type === 'number' ? 'number' : 'text'}
        value={value === undefined || value === null ? '' : String(value)}
      />
    </div>
  )
}

// ---------------------------------------------------------------- prévia

// O campo como quem anota vai ver (dá para mexer: não grava nada).
function Preview({ field }: { field: ProfileField }) {
  const [value, setValue] = useState<FormValue>(() => {
    const d = field.default
    if (Array.isArray(d)) return d.map(String)
    return d === undefined || d === null ? '' : String(d)
  })
  // Enquanto se digita, uma opção pode estar sem valor (ele sai do nome ao
  // sair do campo) ou repetida: a prévia usa o valor que vai ficar e mostra
  // cada um uma vez (o <select> usa o valor como chave; repetido, sobrava
  // opção fantasma na lista).
  const seen = new Set<string>()
  const options = (field.options ?? [])
    .map((o) => ({ ...o, value: o.value || optionValue(o.label ?? '') }))
    .filter((o) => o.value && !seen.has(o.value) && seen.add(o.value))
  const shown: ProfileField = { ...field, options, label: field.label || 'Campo sem nome', key: field.key || 'campo' }
  return (
    <aside aria-label="Prévia na tela de anotação" className="pe-preview">
      <span className="pe-preview__title">Na tela de anotação</span>
      <div className="pe-preview__stage">
        <FieldInput field={shown} onChange={setValue} value={value} />
      </div>
      {field.help && <span className="pe-muted">A ajuda aparece ao parar o mouse no campo.</span>}
    </aside>
  )
}

// ---------------------------------------------------------------- um campo

type FieldCardProps = {
  field: ProfileField
  path: string // "fields[2]"
  index: number
  count: number
  open: boolean
  onToggle: () => void
  onChange: (field: ProfileField) => void
  onMove: (delta: number) => void
  onRemove: () => void
  problems: Problem[]
  readOnly: boolean
}

function FieldCard({ field, path, index, count, open, onToggle, onChange, onMove, onRemove, problems, readOnly }: FieldCardProps) {
  const uid = useId()
  const errors = under(problems, path)
  const own = errors.filter((p) => !p.path.slice(path.length).startsWith('.options'))
  const [touched, setTouched] = useState(false)
  const [editKey, setEditKey] = useState(false)
  const nameMissing = touched && !field.label?.trim()
  const num = (text: string) => (text === '' ? undefined : Number(text))
  const set = (patch: Partial<ProfileField>) => onChange({ ...field, ...patch })
  const body = `${uid}-body`

  return (
    <li className={['pe-card', open ? 'pe-card--open' : '', errors.length ? 'pe-card--error' : ''].join(' ')} id={pathId(path)}>
      <div className="pe-card__bar">
        <button aria-controls={body} aria-expanded={open} className="pe-card__toggle" onClick={onToggle} type="button">
          <span aria-hidden="true" className="pe-chevron" />
          <span className="pe-card__name">{field.label || <em>Campo sem nome</em>}</span>
          <span className="pe-card__meta">{summary(field)}</span>
          {errors.length > 0 && <span className="pe-card__flag">{errors.length === 1 ? '1 problema' : `${errors.length} problemas`}</span>}
        </button>
        {!readOnly && (
          <div className="pe-card__tools">
            <button aria-label={`Subir ${field.label || 'campo'}`} className="pe-icon" disabled={index === 0} onClick={() => onMove(-1)} title="Subir" type="button">↑</button>
            <button aria-label={`Descer ${field.label || 'campo'}`} className="pe-icon" disabled={index === count - 1} onClick={() => onMove(1)} title="Descer" type="button">↓</button>
          </div>
        )}
      </div>

      {open && (
        <div className="pe-card__body" id={body}>
          <div className="pe-card__form">
            <div className="pe-grid">
              <div className={nameMissing ? 'pe-field pe-field--error' : 'pe-field'}>
                <label htmlFor={`${uid}-label`}>Nome na tela</label>
                <input
                  aria-describedby={nameMissing ? `${uid}-label-err` : undefined}
                  aria-invalid={nameMissing || undefined}
                  disabled={readOnly}
                  id={`${uid}-label`}
                  onBlur={() => {
                    setTouched(true)
                    if (!field.key && field.label) set({ key: slug(field.label) })
                  }}
                  onChange={(e) => set({ label: e.target.value })}
                  onFocus={() => setTouched(false)}
                  placeholder="Cor do veículo"
                  value={field.label ?? ''}
                />
                {nameMissing && <small className="pe-error" id={`${uid}-label-err`}>Dê um nome ao campo, como Cor do veículo ou Clima.</small>}
              </div>
              <div className="pe-field">
                <label htmlFor={`${uid}-type`}>Tipo</label>
                <select disabled={readOnly} id={`${uid}-type`} onChange={(e) => onChange(withType(field, e.target.value as FieldType))} value={field.type}>
                  {TYPES.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
              </div>
            </div>

            <div className="pe-switches">
              <label>
                <input checked={Boolean(field.required)} disabled={readOnly} onChange={(e) => set({ required: e.target.checked || undefined })} type="checkbox" />
                <span>Obrigatório para verificar a imagem</span>
              </label>
              <label>
                <input checked={Boolean(field.repeat)} disabled={readOnly} onChange={(e) => set({ repeat: e.target.checked || undefined })} type="checkbox" />
                <span>Repetir o valor da última imagem salva do pacote</span>
              </label>
            </div>

            {(field.type === 'choice' || field.type === 'multi') && (
              <OptionsTable base={path} field={field} onChange={(options) => set({ options })} problems={problems} readOnly={readOnly} />
            )}
            {field.type === 'text' && (
              <div className="pe-grid pe-grid--3">
                <div className="pe-field">
                  <span className="pe-label-row">
                    <label htmlFor={`${uid}-pattern`}>Formato (opcional)</label>
                    <HelpTip label="Formato">Expressão regular para o valor inteiro (o exemplo é uma placa Mercosul).</HelpTip>
                  </span>
                  <input className="pe-mono" disabled={readOnly} id={`${uid}-pattern`} onChange={(e) => set({ pattern: e.target.value || undefined })} placeholder="[A-Z]{3}[0-9][A-Z0-9][0-9]{2}" value={field.pattern ?? ''} />
                </div>
                <div className="pe-field">
                  <label htmlFor={`${uid}-max`}>Tamanho máximo</label>
                  <input disabled={readOnly} id={`${uid}-max`} min={0} onChange={(e) => set({ max_len: num(e.target.value) })} placeholder="7" type="number" value={field.max_len ?? ''} />
                </div>
                <label className="pe-switch-inline">
                  <input checked={Boolean(field.upper)} disabled={readOnly} onChange={(e) => set({ upper: e.target.checked || undefined })} type="checkbox" />
                  <span>Guardar em maiúsculas</span>
                </label>
              </div>
            )}
            {field.type === 'number' && (
              <div className="pe-grid pe-grid--3">
                <div className="pe-field">
                  <label htmlFor={`${uid}-min`}>Mínimo</label>
                  <input disabled={readOnly} id={`${uid}-min`} onChange={(e) => set({ min: num(e.target.value) })} placeholder="2" type="number" value={field.min ?? ''} />
                </div>
                <div className="pe-field">
                  <label htmlFor={`${uid}-maxn`}>Máximo</label>
                  <input disabled={readOnly} id={`${uid}-maxn`} onChange={(e) => set({ max: num(e.target.value) })} placeholder="9" type="number" value={field.max ?? ''} />
                </div>
                <label className="pe-switch-inline">
                  <input checked={Boolean(field.integer)} disabled={readOnly} onChange={(e) => set({ integer: e.target.checked || undefined })} type="checkbox" />
                  <span>Só números inteiros</span>
                </label>
              </div>
            )}
            {field.type === 'ref' && (
              <div className="pe-grid">
                <div className="pe-field">
                  <label htmlFor={`${uid}-ref`}>Qual cadastro</label>
                  <select
                    disabled={readOnly}
                    id={`${uid}-ref`}
                    onChange={(e) => { const ref = e.target.value as 'equipment' | 'camera'; set({ ref, key: ref }) }}
                    value={field.ref ?? 'equipment'}
                  >
                    <option value="equipment">Equipamentos</option>
                    <option value="camera">Câmeras</option>
                  </select>
                </div>
              </div>
            )}

            <div className="pe-grid">
              <DefaultEditor field={field} id={`${uid}-default`} onChange={(value) => set({ default: value })} readOnly={readOnly} />
              <div className="pe-field">
                <label htmlFor={`${uid}-help`}>Ajuda para quem anota (opcional)</label>
                <input disabled={readOnly} id={`${uid}-help`} onChange={(e) => set({ help: e.target.value || undefined })} placeholder="Na dúvida entre preto e cinza, escolha cinza" value={field.help ?? ''} />
              </div>
            </div>

            <div className="pe-key">
              {editKey && !readOnly && field.type !== 'ref' ? (
                <div className="pe-field">
                  <label htmlFor={`${uid}-key`}>Chave no banco</label>
                  <input autoFocus className="pe-mono" id={`${uid}-key`} onBlur={() => setEditKey(false)} onChange={(e) => set({ key: e.target.value.toLowerCase() })} placeholder="cor_do_veiculo" value={field.key} />
                  <small className="pe-muted">Minúsculas, números e _. Mudar a chave de um campo com imagens anotadas não leva os valores junto.</small>
                </div>
              ) : (
                <span className="pe-muted">
                  Chave no banco: <code>{field.key || (field.label ? slug(field.label) : '(sai do nome)')}</code>
                  {!readOnly && field.type !== 'ref' && <> <button className="set-link" onClick={() => setEditKey(true)} type="button">Mudar</button></>}
                </span>
              )}
              {!readOnly && <button className="pe-remove" onClick={onRemove} type="button">Remover campo</button>}
            </div>

            {own.map((p) => <small className="pe-error" key={p.path + p.msg}>{p.path.slice(path.length).replace(/^\./, '') || 'campo'}: {p.msg}</small>)}
          </div>
          <Preview field={field} />
        </div>
      )}
    </li>
  )
}

// ---------------------------------------------------------------- lista de campos

type FieldListProps = {
  base: string // "fields" ou "objects[0].fields"
  fields: ProfileField[] // todos (os herdados com from)
  onChange: (own: ProfileField[]) => void // só os do perfil
  problems: Problem[]
  readOnly: boolean
  empty: ReactNode
  addLabel: string
}

export function FieldList({ base, fields, onChange, problems, readOnly, empty, addLabel }: FieldListProps) {
  const inherited = fields.filter((f) => f.from)
  const own = fields.filter((f) => !f.from)
  // abertos pela pessoa (o índice muda ao subir/descer: o conjunto acompanha)
  const [opened, setOpened] = useState<Set<number>>(() => new Set())
  const isOpen = (i: number) => opened.has(i) || under(problems, `${base}[${i}]`).length > 0
  const toggle = (i: number) => setOpened((s) => { const n = new Set(s); if (n.has(i)) n.delete(i); else n.add(i); return n })

  const move = (i: number, d: number) => {
    const next = [...own]
    const [item] = next.splice(i, 1)
    next.splice(i + d, 0, item)
    onChange(next)
    setOpened((s) => {
      const n = new Set<number>()
      for (const x of s) n.add(x === i ? i + d : x === i + d ? i : x)
      return n
    })
  }
  const remove = (i: number) => {
    onChange(own.filter((_, j) => j !== i))
    setOpened((s) => new Set([...s].filter((x) => x !== i).map((x) => (x > i ? x - 1 : x))))
  }
  const add = () => {
    onChange([...own, newField()])
    setOpened((s) => new Set(s).add(own.length))
    requestAnimationFrame(() => document.getElementById(pathId(`${base}[${own.length}]`))?.querySelector<HTMLInputElement>('.pe-card__body input')?.focus())
  }

  return (
    <div className="pe-list">
      {inherited.length > 0 && (
        <ul className="pe-inherited" aria-label="Campos herdados">
          {inherited.map((f) => (
            <li key={`${f.from}-${f.key}`}>
              <span className="pe-card__name">{f.label || f.key}</span>
              <span className="pe-card__meta">{summary(f)}</span>
              <span className="pe-tag">de {f.from}</span>
            </li>
          ))}
        </ul>
      )}
      {own.length === 0 && inherited.length === 0 && <div className="pe-empty">{empty}</div>}
      {own.length > 0 && (
        <ul className="pe-cards">
          {own.map((field, i) => (
            <FieldCard
              count={own.length}
              field={field}
              index={i}
              key={i}
              onChange={(f) => onChange(own.map((x, j) => (j === i ? f : x)))}
              onMove={(d) => move(i, d)}
              onRemove={() => remove(i)}
              onToggle={() => toggle(i)}
              open={isOpen(i)}
              path={`${base}[${i}]`}
              problems={problems}
              readOnly={readOnly}
            />
          ))}
        </ul>
      )}
      {!readOnly && <button className="pe-add" onClick={add} type="button">{addLabel}</button>}
    </div>
  )
}

// ---------------------------------------------------------------- tipo de objeto

type ObjectHeadProps = {
  object: ObjectType
  path: string
  locked: boolean
  problems: Problem[]
  onChange: (patch: Partial<ObjectType>) => void
}

// O cabeçalho de um tipo de objeto: nome, de onde vem (caixas ou lista) e quantos.
export function ObjectHead({ object, path, locked, problems, onChange }: ObjectHeadProps) {
  const uid = useId()
  const own = problems.filter((p) => p.path === path || p.path === `${path}.key`)
  const kind = object.box ? 'box' : 'list'
  return (
    <div className="pe-object__head">
      <div className="pe-grid">
        <div className="pe-field">
          <label htmlFor={`${uid}-label`}>Nome na tela</label>
          <input
            disabled={locked}
            id={`${uid}-label`}
            onBlur={() => { if (!object.key && object.label) onChange({ key: slug(object.label) }) }}
            onChange={(e) => onChange({ label: e.target.value })}
            placeholder={object.box ? 'Capacete' : 'Veículo'}
            value={object.label ?? ''}
          />
        </div>
        <div className="pe-field">
          <label htmlFor={`${uid}-key`}>{object.box ? 'Nome da caixa' : 'Chave no banco'}</label>
          <input
            className="pe-mono"
            disabled={locked}
            id={`${uid}-key`}
            onChange={(e) => onChange({ key: e.target.value })}
            placeholder={object.box ? 'capacete' : 'veiculo'}
            value={object.key}
          />
        </div>
      </div>

      <fieldset className="pe-kind" disabled={locked}>
        <legend>De onde vem</legend>
        <label className={kind === 'box' ? 'pe-kind__opt pe-kind__opt--on' : 'pe-kind__opt'}>
          <input checked={kind === 'box'} name={`${uid}-kind`} onChange={() => onChange({ box: true, box_labels: undefined, min: undefined, max: undefined })} type="radio" />
          <span>
            <strong>Caixas marcadas</strong>
            <small>Cada caixa importada com esse nome vira um objeto e ganha os campos abaixo.</small>
          </span>
        </label>
        <label className={kind === 'list' ? 'pe-kind__opt pe-kind__opt--on' : 'pe-kind__opt'}>
          <input checked={kind === 'list'} name={`${uid}-kind`} onChange={() => onChange({ box: undefined })} type="radio" />
          <span>
            <strong>Lista na anotação</strong>
            <small>Quem anota acrescenta os objetos na imagem, como os veículos.</small>
          </span>
        </label>
      </fieldset>

      {!object.box && (
        <div className="pe-grid pe-grid--3">
          <div className="pe-field">
            <label htmlFor={`${uid}-min`}>Mínimo por imagem</label>
            <input disabled={locked} id={`${uid}-min`} min={0} onChange={(e) => onChange({ min: e.target.value === '' ? undefined : Number(e.target.value) })} placeholder="0" type="number" value={object.min ?? ''} />
          </div>
          <div className="pe-field">
            <label htmlFor={`${uid}-max`}>Máximo por imagem</label>
            <input aria-describedby={`${uid}-max-help`} disabled={locked} id={`${uid}-max`} min={0} onChange={(e) => onChange({ max: e.target.value === '' ? undefined : Number(e.target.value) })} placeholder="0" type="number" value={object.max ?? ''} />
            <small className="pe-muted" id={`${uid}-max-help`}>0 ou vazio: sem limite.</small>
          </div>
        </div>
      )}

      {own.map((p) => <small className="pe-error" key={p.path + p.msg}>{p.msg}</small>)}
    </div>
  )
}
