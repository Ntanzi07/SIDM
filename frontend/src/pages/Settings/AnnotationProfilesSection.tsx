import { useEffect, useState } from 'react'
import {
  createAnnotationProfile,
  deleteAnnotationProfile,
  listAnnotationProfiles,
  RequestError,
  updateAnnotationProfile,
  type Problem,
} from '../../services/api'
import type { AnnotationProfile, FieldType, ObjectType, ProfileField, ProfileOption } from '../../types/annotation'
import { useToast, useToastState } from '../../hooks/useToast'
import { confirmDialog } from '../../services/confirm'
import { BoxLabelsEditor } from './BoxLabelsEditor'
import { applyParent, ownFields, ownIndex, parentChoices } from './profileInheritance'

// Editor dos PERFIS DE ANOTAÇÃO: o que se anota em cada imagem de um pacote.
// Quem confere tudo é o backend (POST/PUT /annotation-profiles): os erros
// voltam juntos, cada um com o caminho ("fields[2].options[1].code"), e a
// linha do campo com problema fica marcada.

const TYPES: Array<[FieldType, string]> = [
  ['choice', 'Lista (um valor)'],
  ['multi', 'Lista (vários valores)'],
  ['text', 'Texto'],
  ['number', 'Número'],
  ['bool', 'Sim/não'],
  ['datetime', 'Data e hora'],
  ['ref', 'Cadastro (equipamento/câmera)'],
]

// "Cor do cabelo" -> "cor_do_cabelo" (o formato que o backend aceita).
function slug(text: string): string {
  const s = text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 32)
  return /^[a-z]/.test(s) ? s : s ? `c_${s}`.slice(0, 32) : ''
}

const optionValue = (text: string) => slug(text).replace(/_/g, '-').replace(/^c-/, '') || ''

function newField(): ProfileField {
  return { key: '', label: '', type: 'choice', options: [] }
}

// Ao trocar o tipo, some o que só valia para o tipo antigo.
function withType(field: ProfileField, type: FieldType): ProfileField {
  const base: ProfileField = { key: field.key, label: field.label, type, required: field.required, help: field.help, repeat: field.repeat }
  if (type === 'choice' || type === 'multi') base.options = field.options ?? []
  if (type === 'ref') {
    base.ref = 'equipment'
    base.key = 'equipment' // o backend exige: a cascata de renomear procura por esse nome
  }
  return base
}

// Problemas de um caminho e de tudo abaixo dele.
const under = (problems: Problem[], path: string) =>
  problems.filter((p) => p.path === path || p.path.startsWith(`${path}.`) || p.path.startsWith(`${path}[`))

type FieldsEditorProps = {
  base: string // "fields" ou "objects[0].fields"
  fields: ProfileField[]
  onChange: (fields: ProfileField[]) => void
  problems: Problem[]
  readOnly: boolean
}

function OptionsEditor({ field, onChange, readOnly }: { field: ProfileField; onChange: (options: ProfileOption[]) => void; readOnly: boolean }) {
  const options = field.options ?? []
  const set = (i: number, patch: Partial<ProfileOption>) => onChange(options.map((o, j) => (j === i ? { ...o, ...patch } : o)))
  return (
    <div className="ap-options">
      <span className="ap-small">Opções (valor guardado · nome na tela · código no nome do arquivo, opcional)</span>
      {options.map((option, i) => (
        <div className="ap-option" key={i}>
          <input disabled={readOnly} onChange={(e) => set(i, { value: e.target.value })} placeholder="valor (ex.: black)" value={option.value} />
          <input
            disabled={readOnly}
            onBlur={() => { if (!option.value && option.label) set(i, { value: optionValue(option.label) }) }}
            onChange={(e) => set(i, { label: e.target.value })}
            placeholder="nome (ex.: Preto)"
            value={option.label ?? ''}
          />
          <input className="ap-code" disabled={readOnly} onChange={(e) => set(i, { code: e.target.value.toUpperCase() })} placeholder="código" value={option.code ?? ''} />
          {!readOnly && <button className="button button--danger button--sm" onClick={() => onChange(options.filter((_, j) => j !== i))} type="button">✕</button>}
        </div>
      ))}
      {!readOnly && (
        <button className="button button--secondary button--sm" onClick={() => onChange([...options, { value: '', label: '' }])} type="button">+ Opção</button>
      )}
    </div>
  )
}

// O valor inicial: o que a tela de anotação já traz numa imagem sem rascunho.
function DefaultEditor({ field, onChange, readOnly }: { field: ProfileField; onChange: (value: unknown) => void; readOnly: boolean }) {
  const value = field.default
  if (field.type === 'ref' || field.type === 'datetime') return null
  if (field.type === 'choice' || field.type === 'bool') {
    const options = field.type === 'bool' ? [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }] : field.options ?? []
    const current = value === undefined || value === null ? '' : String(value)
    return (
      <label className="ap-inline">
        <span>Valor inicial</span>
        <select
          disabled={readOnly}
          onChange={(e) => onChange(e.target.value === '' ? undefined : field.type === 'bool' ? e.target.value === 'true' : e.target.value)}
          value={current}
        >
          <option value="">(nenhum)</option>
          {options.map((o) => <option key={o.value} value={o.value}>{o.label || o.value}</option>)}
        </select>
      </label>
    )
  }
  if (field.type === 'multi') {
    const chosen = Array.isArray(value) ? value.map(String) : []
    return (
      <div className="ap-inline">
        <span>Marcadas no início</span>
        <div className="ap-checks">
          {(field.options ?? []).filter((o) => o.value).map((o) => (
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
        </div>
      </div>
    )
  }
  return (
    <label className="ap-inline">
      <span>Valor inicial</span>
      <input
        disabled={readOnly}
        onChange={(e) => {
          const text = e.target.value
          onChange(text === '' ? undefined : field.type === 'number' ? Number(text) : text)
        }}
        type={field.type === 'number' ? 'number' : 'text'}
        value={value === undefined || value === null ? '' : String(value)}
      />
    </label>
  )
}

function FieldsEditor({ base, fields, onChange, problems, readOnly }: FieldsEditorProps) {
  const set = (i: number, field: ProfileField) => onChange(fields.map((f, j) => (j === i ? field : f)))
  const move = (i: number, d: number) => {
    const next = [...fields]
    const [item] = next.splice(i, 1)
    next.splice(i + d, 0, item)
    onChange(next)
  }
  const num = (text: string) => (text === '' ? undefined : Number(text))

  return (
    <div className="ap-fields">
      {fields.length === 0 && <p className="ap-small">Nenhum campo.</p>}
      {fields.map((field, i) => {
        const path = `${base}[${i}]`
        const errors = under(problems, path)
        return (
          <div className={errors.length ? 'ap-field ap-field--error' : 'ap-field'} key={i}>
            <div className="ap-row">
              <label className="ap-inline">
                <span>Nome na tela</span>
                <input
                  disabled={readOnly}
                  onBlur={() => { if (!field.key && field.label) set(i, { ...field, key: slug(field.label) }) }}
                  onChange={(e) => set(i, { ...field, label: e.target.value })}
                  placeholder="ex.: Cor do cabelo"
                  value={field.label ?? ''}
                />
              </label>
              <label className="ap-inline">
                <span>Chave (no banco)</span>
                <input
                  disabled={readOnly || field.type === 'ref'}
                  onChange={(e) => set(i, { ...field, key: e.target.value })}
                  placeholder="ex.: cor_cabelo"
                  value={field.key}
                />
              </label>
              <label className="ap-inline">
                <span>Tipo</span>
                <select disabled={readOnly} onChange={(e) => set(i, withType(field, e.target.value as FieldType))} value={field.type}>
                  {TYPES.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
              </label>
              <label className="ap-check">
                <input checked={Boolean(field.required)} disabled={readOnly} onChange={(e) => set(i, { ...field, required: e.target.checked || undefined })} type="checkbox" />
                Obrigatório
              </label>
              {!readOnly && (
                <div className="ap-row-actions">
                  <button className="button button--secondary button--sm" disabled={i === 0} onClick={() => move(i, -1)} title="Subir" type="button">↑</button>
                  <button className="button button--secondary button--sm" disabled={i === fields.length - 1} onClick={() => move(i, 1)} title="Descer" type="button">↓</button>
                  <button className="button button--danger button--sm" onClick={() => onChange(fields.filter((_, j) => j !== i))} type="button">Remover</button>
                </div>
              )}
            </div>

            {(field.type === 'choice' || field.type === 'multi') && (
              <OptionsEditor field={field} onChange={(options) => set(i, { ...field, options })} readOnly={readOnly} />
            )}
            {field.type === 'text' && (
              <div className="ap-row">
                <label className="ap-inline">
                  <span>Formato (expressão regular, opcional)</span>
                  <input disabled={readOnly} onChange={(e) => set(i, { ...field, pattern: e.target.value || undefined })} placeholder="ex.: [A-Z0-9]{7}" value={field.pattern ?? ''} />
                </label>
                <label className="ap-inline">
                  <span>Tamanho máximo</span>
                  <input disabled={readOnly} min={0} onChange={(e) => set(i, { ...field, max_len: num(e.target.value) })} type="number" value={field.max_len ?? ''} />
                </label>
                <label className="ap-check">
                  <input checked={Boolean(field.upper)} disabled={readOnly} onChange={(e) => set(i, { ...field, upper: e.target.checked || undefined })} type="checkbox" />
                  Guardar em maiúsculas
                </label>
              </div>
            )}
            {field.type === 'number' && (
              <div className="ap-row">
                <label className="ap-inline">
                  <span>Mínimo</span>
                  <input disabled={readOnly} onChange={(e) => set(i, { ...field, min: num(e.target.value) })} type="number" value={field.min ?? ''} />
                </label>
                <label className="ap-inline">
                  <span>Máximo</span>
                  <input disabled={readOnly} onChange={(e) => set(i, { ...field, max: num(e.target.value) })} type="number" value={field.max ?? ''} />
                </label>
                <label className="ap-check">
                  <input checked={Boolean(field.integer)} disabled={readOnly} onChange={(e) => set(i, { ...field, integer: e.target.checked || undefined })} type="checkbox" />
                  Só inteiros
                </label>
              </div>
            )}
            {field.type === 'ref' && (
              <div className="ap-row">
                <label className="ap-inline">
                  <span>Qual cadastro</span>
                  <select
                    disabled={readOnly}
                    onChange={(e) => { const ref = e.target.value as 'equipment' | 'camera'; set(i, { ...field, ref, key: ref }) }}
                    value={field.ref ?? 'equipment'}
                  >
                    <option value="equipment">Equipamentos</option>
                    <option value="camera">Câmeras</option>
                  </select>
                </label>
              </div>
            )}
            <div className="ap-row">
              <DefaultEditor field={field} onChange={(value) => set(i, { ...field, default: value })} readOnly={readOnly} />
              <label className="ap-inline ap-grow">
                <span>Ajuda (aparece no campo)</span>
                <input disabled={readOnly} onChange={(e) => set(i, { ...field, help: e.target.value || undefined })} value={field.help ?? ''} />
              </label>
            </div>
            {errors.map((p) => <small className="ap-error" key={p.path + p.msg}>{p.path.slice(path.length).replace(/^\./, '') || 'campo'}: {p.msg}</small>)}
          </div>
        )
      })}
      {!readOnly && (
        <button className="button button--secondary button--sm" onClick={() => onChange([...fields, newField()])} type="button">+ Campo</button>
      )}
    </div>
  )
}

// Os campos que vieram do pai, só para ver (mudam no perfil de onde vieram).
function Inherited({ fields }: { fields: ProfileField[] }) {
  const inherited = fields.filter((f) => f.from)
  if (inherited.length === 0) return null
  const from = [...new Set(inherited.map((f) => f.from))].join(', ')
  return (
    <div className="ap-inherited">
      <span>Herdados de {from}: {inherited.map((f) => f.label || f.key).join(', ')}</span>
    </div>
  )
}

const empty = (): AnnotationProfile => ({ name: '', description: '', fields: [], objects: [] })

// A seção tem duas telas no menu de Configurar, como os perfis de import:
// a lista (com a quantidade no menu) e o editor ("Criar novo perfil"). Ela
// fica montada o tempo todo (view null = escondida) para o rascunho não se
// perder ao trocar de tela.
type View = 'list' | 'editor'
type Props = {
  view: View | null
  onView: (view: View) => void
  onSummary: (summary: { count: number; editorLabel: string }) => void
}

export function AnnotationProfilesSection({ view, onView, onSummary }: Props) {
  const [profiles, setProfiles] = useState<AnnotationProfile[]>([])
  const [draft, setDraft] = useState<AnnotationProfile | null>(null)
  const [isNew, setIsNew] = useState(false)
  const [problems, setProblems] = useState<Problem[]>([])
  const [error, setError] = useState('')
  useToast(error && !problems.length ? error : '', 'error')
  const [, setMessage] = useToastState('success')
  const [saving, setSaving] = useState(false)

  const load = () => listAnnotationProfiles().then(setProfiles).catch((e: unknown) => setError(e instanceof Error ? e.message : 'Não foi possível carregar os perfis.'))
  useEffect(() => { void load() }, [])

  const readOnly = Boolean(draft?.builtin)

  // O menu mostra a quantidade e o nome do editor ("Ver", "Editar"...).
  const editorLabel = !draft || isNew ? 'Criar novo perfil' : readOnly ? 'Ver perfil' : 'Editar perfil'
  useEffect(() => { onSummary({ count: profiles.length, editorLabel }) }, [profiles.length, editorLabel, onSummary])

  // Abriu "Criar novo perfil" pelo menu sem nada aberto: um perfil em branco.
  if (view === 'editor' && !draft) {
    setDraft(empty())
    setIsNew(true)
  }

  function close() {
    setDraft(null)
    setProblems([])
    onView('list')
  }

  function open(profile: AnnotationProfile, asNew: boolean) {
    setDraft(structuredClone(profile))
    setIsNew(asNew)
    setProblems([])
    setError('')
    setMessage('')
    onView('editor')
  }

  // Uma cópia editável (do padrão, por exemplo): o nome muda e sai o "padrão".
  function duplicate(profile: AnnotationProfile) {
    const copy = structuredClone(profile)
    open({ ...copy, name: `${profile.name}-copia`, builtin: undefined, created_by: undefined, updated_at: undefined, lineage: undefined }, true)
  }

  async function save() {
    if (!draft) return
    setSaving(true)
    setProblems([])
    setError('')
    setMessage('')
    try {
      const saved = isNew ? await createAnnotationProfile(draft) : await updateAnnotationProfile(draft)
      // Como nos perfis de import: salvou, volta para a lista.
      close()
      setMessage(`Perfil "${saved.name}" ${isNew ? 'criado' : 'salvo'}.`)
      await load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Não foi possível salvar o perfil.')
      if (e instanceof RequestError) setProblems(e.problems)
    } finally {
      setSaving(false)
    }
  }

  async function remove(name: string) {
    // Em uso por algum pacote, o backend recusa e diz quais.
    if (!(await confirmDialog({
      title: 'Apagar perfil de anotação?',
      message: `O perfil "${name}" será apagado. Se algum pacote usar este perfil, ele não é apagado.`,
      confirmLabel: 'Apagar perfil',
      tone: 'danger',
    }))) return
    try {
      await deleteAnnotationProfile(name)
      setDraft(null)
      setMessage(`Perfil "${name}" apagado.`)
      await load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Não foi possível apagar o perfil.')
    }
  }

  const setObject = (i: number, patch: Partial<ObjectType>) =>
    draft && setDraft({ ...draft, objects: draft.objects.map((o, j) => (j === i ? { ...o, ...patch } : o)) })

  if (view === null) return null

  const notices = (
    <>
    </>
  )

  if (view === 'list') return (
    <div className="create-card settings-card">
      <div className="profile-header-flex">
        <div>
          <h2>Perfis de anotação cadastrados</h2>
          <p>O que se anota em cada imagem de um pacote: os campos da imagem e os tipos de objeto.</p>
        </div>
        <button className="button button--primary button--sm" onClick={() => open(empty(), true)} type="button">+ Novo perfil</button>
      </div>
      {notices}

      <div className="profile-items-list">
        {profiles.map((p) => (
          <div className={`profile-item ${draft?.name === p.name && !isNew ? 'profile-item--selected' : ''}`} key={p.name}>
            <div className="profile-item__header">
              <div>
                <strong className="profile-item__name">
                  {p.name}
                  {p.builtin && <span className="profile-type-badge">padrão</span>}
                  {p.extends && <span className="profile-type-badge">estende {p.extends}</span>}
                </strong>
                {p.description && <span className="profile-item__desc">{p.description}</span>}
              </div>
              <div className="profile-item__actions">
                <button className="button button--secondary button--sm" onClick={() => open(p, false)} type="button">{p.builtin ? 'Abrir' : 'Editar'}</button>
                <button className="button button--secondary button--sm" onClick={() => duplicate(p)} type="button">Duplicar</button>
                {!p.builtin && <button className="button button--danger button--sm" onClick={() => void remove(p.name)} type="button">Apagar</button>}
              </div>
            </div>
            <div className="profile-item__details">
              <span><strong>Campos:</strong> {p.fields.length}</span>
              <span><strong>Objetos:</strong> {p.objects.map((o) => `${o.label || o.key}${o.box ? ' (caixa)' : ''}`).join(', ') || 'nenhum'}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )

  return (
    <div className="create-card settings-card">
      {notices}
      {draft && (
        <div className="ap-editor">
          <div className="profile-header-flex">
            <div>
              <h2>{isNew ? 'Criar novo perfil de anotação' : readOnly ? `Perfil padrão: ${draft.name}` : `Editar perfil: ${draft.name}`}</h2>
              <p>{readOnly ? 'Perfil padrão: aqui só mudam as caixas ligadas a cada tipo de objeto. Para mudar o resto, use Duplicar.' : 'Os campos da imagem e os tipos de objeto que se anotam nos pacotes deste perfil.'}</p>
            </div>
            {readOnly && (
              <div className="profile-item__actions">
                <button className="button button--secondary button--sm" onClick={() => duplicate(draft)} type="button">Duplicar</button>
                <button className="button button--secondary button--sm" onClick={close} type="button">Fechar</button>
              </div>
            )}
          </div>
          <div className="ap-row">
            <label className="ap-inline">
              <span>Nome</span>
              <input
                className={under(problems, 'name').length ? 'ap-input--error' : ''}
                disabled={!isNew}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder="ex.: capacete-moto"
                value={draft.name}
              />
            </label>
            <label className="ap-inline ap-grow">
              <span>Descrição</span>
              <input disabled={readOnly} onChange={(e) => setDraft({ ...draft, description: e.target.value })} value={draft.description ?? ''} />
            </label>
          </div>
          {under(problems, 'name').map((p) => <small className="ap-error" key={p.msg}>nome: {p.msg}</small>)}
          <div className="ap-row">
            <label className="ap-inline">
              <span>Estende (herda os campos e os tipos de)</span>
              <select
                disabled={readOnly}
                onChange={(e) => setDraft(applyParent(draft, profiles.find((p) => p.name === e.target.value)))}
                value={draft.extends ?? ''}
              >
                <option value="">Nenhum</option>
                {parentChoices(profiles, isNew ? '' : draft.name).map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}
              </select>
            </label>
            {draft.extends && (
              <p className="ap-small ap-grow">
                Os pacotes deste perfil entram nos datasets e na pesquisa de {draft.extends}. O herdado aparece só para
                leitura: muda no perfil {draft.extends}.
              </p>
            )}
          </div>
          {under(problems, 'extends').map((p) => <small className="ap-error" key={p.msg}>estende: {p.msg}</small>)}

          <h4>Campos da imagem</h4>
          <Inherited fields={draft.fields} />
          <FieldsEditor
            base="fields"
            fields={ownFields(draft.fields)}
            onChange={(fields) => setDraft({ ...draft, fields: [...draft.fields.filter((f) => f.from), ...fields] })}
            problems={problems}
            readOnly={readOnly}
          />

          <h4>Tipos de objeto</h4>
          <p className="ap-small">
            Com caixa: o objeto é uma label do CVAT (cada caixa importada ganha os campos). Sem caixa: uma lista na
            imagem, preenchida na tela de anotação (como os veículos).
          </p>
          {draft.objects.map((object, i) => {
            const path = `objects[${ownIndex(draft.objects, i)}]`
            const own = problems.filter((p) => p.path === path || p.path === `${path}.key`)
            const locked = readOnly || Boolean(object.from)
            return (
              <div className={own.length ? 'ap-object ap-field--error' : 'ap-object'} key={i}>
                <div className="ap-row">
                  <label className="ap-inline">
                    <span>Nome na tela</span>
                    <input
                      disabled={locked}
                      onBlur={() => { if (!object.key && object.label) setObject(i, { key: slug(object.label) }) }}
                      onChange={(e) => setObject(i, { label: e.target.value })}
                      placeholder="ex.: Capacete"
                      value={object.label ?? ''}
                    />
                  </label>
                  <label className="ap-inline">
                    <span>Chave (label do CVAT, se tiver caixa)</span>
                    <input disabled={locked} onChange={(e) => setObject(i, { key: e.target.value })} placeholder="ex.: helmet" value={object.key} />
                  </label>
                  <label className="ap-check">
                    <input checked={Boolean(object.box)} disabled={locked} onChange={(e) => setObject(i, { box: e.target.checked || undefined, box_labels: e.target.checked ? undefined : object.box_labels })} type="checkbox" />
                    Tem caixa (CVAT)
                  </label>
                  {!object.box && (
                    <>
                      <label className="ap-inline ap-num">
                        <span>Mínimo</span>
                        <input disabled={locked} min={0} onChange={(e) => setObject(i, { min: e.target.value === '' ? undefined : Number(e.target.value) })} type="number" value={object.min ?? ''} />
                      </label>
                      <label className="ap-inline ap-num">
                        <span>Máximo (0 = sem limite)</span>
                        <input disabled={locked} min={0} onChange={(e) => setObject(i, { max: e.target.value === '' ? undefined : Number(e.target.value) })} type="number" value={object.max ?? ''} />
                      </label>
                    </>
                  )}
                  {!locked && (
                    <div className="ap-row-actions">
                      <button className="button button--danger button--sm" onClick={() => setDraft({ ...draft, objects: draft.objects.filter((_, j) => j !== i) })} type="button">Remover</button>
                    </div>
                  )}
                  {object.from && <span className="ap-from">tipo de {object.from}: aqui dá para acrescentar campos e caixas</span>}
                </div>
                {own.map((p) => <small className="ap-error" key={p.path + p.msg}>{p.msg}</small>)}
                <Inherited fields={object.fields} />
                <FieldsEditor
                  base={`${path}.fields`}
                  fields={ownFields(object.fields)}
                  onChange={(fields) => setObject(i, { fields: [...object.fields.filter((f) => f.from), ...fields] })}
                  problems={problems}
                  readOnly={readOnly}
                />
                {!object.box && (
                  <BoxLabelsEditor
                    object={object}
                    onChange={(box_labels) => setObject(i, { box_labels: box_labels.length ? box_labels : undefined })}
                    problems={under(problems, `objects[${i}].box_labels`)}
                    readOnly={false} // até num padrão: labels novas aparecem com tarefas novas
                  />
                )}
              </div>
            )
          })}
          {!readOnly && (
            <button
              className="button button--secondary button--sm"
              onClick={() => setDraft({ ...draft, objects: [...draft.objects, { key: '', label: '', fields: [] }] })}
              type="button"
            >
              + Tipo de objeto
            </button>
          )}

          {problems.length > 0 && (
            <div className="ap-problems">
              <strong>{problems.length === 1 ? '1 problema' : `${problems.length} problemas`} (as linhas estão marcadas):</strong>
              <ul>{problems.map((p) => <li key={p.path + p.msg}><code>{p.path}</code>: {p.msg}</li>)}</ul>
            </div>
          )}

          <div className="profile-submit-row">
            <button className="button button--secondary" onClick={close} type="button">{isNew ? 'Cancelar' : 'Fechar'}</button>
            {(!readOnly || draft.builtin) && (
              <button className="button button--primary" disabled={saving} onClick={() => void save()} type="button">
                {saving ? 'Salvando...' : isNew ? 'Criar perfil' : 'Salvar perfil'}
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
