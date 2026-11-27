import { useEffect, useId, useState } from 'react'
import {
  createAnnotationProfile,
  deleteAnnotationProfile,
  listAnnotationProfiles,
  RequestError,
  updateAnnotationProfile,
  type Problem,
} from '../../services/api'
import type { AnnotationProfile, ObjectType } from '../../types/annotation'
import { useToast, useToastState } from '../../hooks/useToast'
import { confirmDialog } from '../../services/confirm'
import { BoxLabelsEditor } from './BoxLabelsEditor'
import { HelpTip } from '../../components/HelpTip/HelpTip'
import { applyParent, ownFields, ownIndex, parentChoices } from './profileInheritance'
import { FieldList, ObjectHead } from './ProfileEditor'
import { pathId, profileName, under } from './profileEditorUtils'
import { ProfileList } from './ProfileList'
import { SettingRow, SettingsGroup, SettingsHeader } from './SettingsUI'

// Os PERFIS DE ANOTAÇÃO: o que se anota em cada imagem de um pacote. A
// lista e o editor são duas telas no menu do Configurar; a seção fica
// montada o tempo todo (view null = escondida) para o rascunho não se perder
// ao trocar de tela. As peças do editor estão em ProfileEditor.tsx.

const empty = (): AnnotationProfile => ({ name: '', description: '', fields: [], objects: [] })

// "3 campos" / "1 campo"
const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

type View = 'list' | 'editor'
type Props = {
  view: View | null
  onView: (view: View) => void
  onSummary: (summary: { count: number; editorLabel: string }) => void
}

export function AnnotationProfilesSection({ view, onView, onSummary }: Props) {
  const uid = useId()
  const [profiles, setProfiles] = useState<AnnotationProfile[]>([])
  const [draft, setDraft] = useState<AnnotationProfile | null>(null)
  const [snapshot, setSnapshot] = useState('') // o rascunho como abriu (para saber se mudou)
  const [isNew, setIsNew] = useState(false)
  const [problems, setProblems] = useState<Problem[]>([])
  const [error, setError] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  useToast(error && !problems.length ? error : '', 'error')
  const [, setMessage] = useToastState('success')
  const [saving, setSaving] = useState(false)

  const load = () => listAnnotationProfiles().then(setProfiles).catch((e: unknown) => setError(e instanceof Error ? e.message : 'Não foi possível carregar os perfis.'))
  useEffect(() => { void load() }, [])

  const readOnly = Boolean(draft?.builtin)
  const dirty = draft !== null && JSON.stringify(draft) !== snapshot

  // O menu mostra a quantidade e o nome do editor ("Ver", "Editar"...).
  const editorLabel = !draft || isNew ? 'Criar novo perfil' : readOnly ? 'Ver perfil' : 'Editar perfil'
  useEffect(() => { onSummary({ count: profiles.length, editorLabel }) }, [profiles.length, editorLabel, onSummary])

  // só o estado daqui (dá para chamar durante a renderização)
  function start(profile: AnnotationProfile, asNew: boolean) {
    const copy = structuredClone(profile)
    setDraft(copy)
    setSnapshot(JSON.stringify(copy))
    setIsNew(asNew)
    setProblems([])
    setError('')
    setNameTouched(false)
  }

  function open(profile: AnnotationProfile, asNew: boolean) {
    start(profile, asNew)
    setMessage('')
    onView('editor')
  }

  // Abriu "Criar novo perfil" pelo menu sem nada aberto: um perfil em branco.
  if (view === 'editor' && !draft) start(empty(), true)

  function reset() {
    setDraft(null)
    setSnapshot('')
    setProblems([])
    onView('list')
  }

  // Fechar com mudança pergunta antes (o rascunho se perderia).
  async function close() {
    if (dirty && !(await confirmDialog({
      title: 'Descartar as alterações?',
      message: isNew ? 'O perfil ainda não foi criado. O que foi preenchido se perde.' : `As mudanças no perfil "${draft?.name}" ainda não foram salvas.`,
      confirmLabel: 'Descartar alterações',
      tone: 'danger',
    }))) return
    reset()
  }

  // Uma cópia editável (do padrão, por exemplo): o nome muda e sai o "padrão".
  function duplicate(profile: AnnotationProfile) {
    const copy = structuredClone(profile)
    open({ ...copy, name: `${profile.name}-copia`, builtin: undefined, created_by: undefined, updated_at: undefined, lineage: undefined }, true)
  }

  async function save() {
    if (!draft) return
    // o nome é o único obrigatório que dá para conferir aqui; o resto o backend diz
    if (isNew && !draft.name) {
      setNameTouched(true)
      document.getElementById(`${uid}-name`)?.focus()
      return
    }
    setSaving(true)
    setProblems([])
    setError('')
    setMessage('')
    try {
      const saved = isNew ? await createAnnotationProfile(draft) : await updateAnnotationProfile(draft)
      // Como nos perfis de import: salvou, volta para a lista.
      reset()
      setMessage(`Perfil "${saved.name}" ${isNew ? 'criado' : 'salvo'}.`)
      await load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Não foi possível salvar o perfil.')
      if (e instanceof RequestError) {
        setProblems(e.problems)
        // leva até o primeiro problema (o campo dele já abre sozinho)
        const first = e.problems[0]?.path.match(/^(objects\[\d+\]\.)?fields\[\d+\]/)?.[0] ?? e.problems[0]?.path.match(/^objects\[\d+\]/)?.[0]
        requestAnimationFrame(() => (first ? document.getElementById(pathId(first)) : document.getElementById(`${uid}-problems`))?.scrollIntoView({ block: 'center', behavior: 'smooth' }))
      }
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
      setMessage(`Perfil "${name}" apagado.`)
      await load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Não foi possível apagar o perfil.')
    }
  }

  if (view === null) return null

  // ------------------------------------------------------------ a lista
  if (view === 'list') return (
    <div className="set-page">
      <SettingsHeader
        actions={<button className="button button--primary button--sm" onClick={() => open(empty(), true)} type="button">Criar perfil</button>}
        info={(
          <ul>
            <li>Cada pacote usa um perfil: o que se anota em cada imagem.</li>
            <li>Um perfil pode estender outro: herda tudo e acrescenta o que falta.</li>
          </ul>
        )}
        title="Perfis de anotação"
      />
      <ProfileList onDuplicate={duplicate} onOpen={(p) => open(p, false)} onRemove={(name) => void remove(name)} profiles={profiles} />
    </div>
  )

  if (!draft) return null

  // ------------------------------------------------------------ o editor
  const setObject = (i: number, patch: Partial<ObjectType>) =>
    setDraft({ ...draft, objects: draft.objects.map((o, j) => (j === i ? { ...o, ...patch } : o)) })
  const addObject = () => {
    setDraft({ ...draft, objects: [...draft.objects, { key: '', label: '', fields: [] }] })
    requestAnimationFrame(() => document.querySelector<HTMLInputElement>('.pe-object:last-of-type input')?.focus())
  }
  const nameMissing = isNew && nameTouched && !draft.name
  const nameProblems = under(problems, 'name')
  const ownFieldCount = ownFields(draft.fields).length

  return (
    <div className="set-page pe-editor">
      <SettingsHeader
        actions={readOnly ? <button className="button button--secondary button--sm" onClick={() => duplicate(draft)} type="button">Duplicar para editar</button> : undefined}
        description={readOnly ? 'Perfil padrão do SIDM: só as caixas de cada tipo de objeto mudam. Para o resto, duplique.' : undefined}
        info={readOnly ? undefined : 'Abra um campo para editar e ver como ele fica na anotação.'}
        title={isNew ? 'Criar perfil de anotação' : readOnly ? `Perfil ${draft.name}` : `Editar ${draft.name}`}
      />

      <SettingsGroup title="Sobre o perfil">
        <SettingRow
          info={isNew ? 'Minúsculas, números e hífen. Não muda depois.' : 'Não muda depois de criado. Para outro nome, duplique.'}
          htmlFor={`${uid}-name`}
          label="Nome"
        >
          <div className={nameMissing || nameProblems.length ? 'pe-field pe-field--error' : 'pe-field'}>
            <input
              aria-describedby={nameMissing || nameProblems.length ? `${uid}-name-err` : undefined}
              aria-invalid={nameMissing || nameProblems.length > 0 || undefined}
              className="pe-mono pe-w-md"
              disabled={!isNew}
              id={`${uid}-name`}
              onBlur={() => { setNameTouched(true); setDraft({ ...draft, name: draft.name.replace(/-+$/, '') }) }}
              onChange={(e) => setDraft({ ...draft, name: profileName(e.target.value) })}
              onFocus={() => setNameTouched(false)}
              placeholder="caminhoes"
              value={draft.name}
            />
            {(nameMissing || nameProblems.length > 0) && (
              <small className="pe-error" id={`${uid}-name-err`}>{nameMissing ? 'Dê um nome ao perfil, como caminhoes.' : nameProblems.map((p) => p.msg).join(' ')}</small>
            )}
          </div>
        </SettingRow>
        <SettingRow htmlFor={`${uid}-desc`} info="Aparece na lista e ao escolher o perfil de um pacote." label="Descrição">
          <input
            className="pe-w-lg"
            disabled={readOnly}
            id={`${uid}-desc`}
            onChange={(e) => setDraft({ ...draft, description: e.target.value })}
            placeholder="Caminhões: eixos e tipo de carroceria"
            value={draft.description ?? ''}
          />
        </SettingRow>
        <SettingRow
          info={draft.extends
            ? (
              <ul>
                <li>Herda os campos e os tipos de {draft.extends} (só leitura aqui).</li>
                <li>Os pacotes deste perfil entram nos datasets e na pesquisa de {draft.extends}.</li>
              </ul>
            )
            : 'Herdar traz os campos e os tipos do outro perfil; aqui você só acrescenta.'}
          htmlFor={`${uid}-extends`}
          label="Estende"
        >
          <select
            className="pe-w-md"
            disabled={readOnly}
            id={`${uid}-extends`}
            onChange={(e) => setDraft(applyParent(draft, profiles.find((p) => p.name === e.target.value)))}
            value={draft.extends ?? ''}
          >
            <option value="">Nenhum (começa do zero)</option>
            {parentChoices(profiles, isNew ? '' : draft.name).map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}
          </select>
        </SettingRow>
        {under(problems, 'extends').map((p) => <small className="pe-error pe-pad" key={p.msg}>{p.msg}</small>)}
      </SettingsGroup>

      <section className="set-group" aria-labelledby={`${uid}-fields`}>
        <div className="set-group__head">
          <h3 id={`${uid}-fields`}>Campos da imagem<HelpTip label="Campos da imagem">Preenchidos uma vez por imagem, como o clima, a câmera ou o ângulo.</HelpTip></h3>
        </div>
        <FieldList
          addLabel={ownFieldCount ? 'Adicionar campo' : 'Adicionar o primeiro campo'}
          base="fields"
          empty={<><strong>Nenhum campo ainda</strong><span>Por exemplo: Clima (lista com Sol, Chuva, Noite) ou Placa legível (sim/não).</span></>}
          fields={draft.fields}
          onChange={(own) => setDraft({ ...draft, fields: [...draft.fields.filter((f) => f.from), ...own] })}
          problems={problems}
          readOnly={readOnly}
        />
      </section>

      <section className="set-group" aria-labelledby={`${uid}-objects`}>
        <div className="set-group__head">
          <h3 id={`${uid}-objects`}>Tipos de objeto<HelpTip label="Tipos de objeto">O que aparece dentro da imagem e tem campos próprios, como um veículo ou um capacete.</HelpTip></h3>
        </div>
        {draft.objects.length === 0 && (
          <div className="pe-empty">
            <strong>Nenhum tipo de objeto</strong>
            <span>Um perfil pode ter só campos da imagem. Acrescente um tipo quando cada imagem tiver vários itens para anotar.</span>
          </div>
        )}
        {draft.objects.map((object, i) => {
          const path = `objects[${ownIndex(draft.objects, i)}]`
          const locked = readOnly || Boolean(object.from)
          return (
            <article aria-label={object.label || object.key || 'Tipo de objeto sem nome'} className="pe-object" id={pathId(path)} key={i}>
              <header className="pe-object__title">
                <h4>{object.label || object.key || <em>Tipo sem nome</em>}</h4>
                <span className="pe-card__meta">
                  {object.box ? 'caixas marcadas' : 'lista na anotação'}, {count(object.fields.length, 'campo', 'campos')}
                </span>
                {object.from && <span className="pe-tag">de {object.from}: aqui só se acrescentam campos e caixas</span>}
                {!locked && (
                  <button className="pe-remove pe-object__remove" onClick={() => setDraft({ ...draft, objects: draft.objects.filter((_, j) => j !== i) })} type="button">
                    Remover tipo
                  </button>
                )}
              </header>
              <ObjectHead
                locked={locked}
                object={object}
                onChange={(patch) => setObject(i, patch)}
                path={path}
                problems={problems}
              />
              <div className="pe-object__fields">
                <h5>Campos de cada {(object.label || 'objeto').toLowerCase()}</h5>
                <FieldList
                  addLabel="Adicionar campo"
                  base={`${path}.fields`}
                  empty={<span>Nenhum campo. Por exemplo: Cor (lista) ou Tem garupa (sim/não).</span>}
                  fields={object.fields}
                  onChange={(own) => setObject(i, { fields: [...object.fields.filter((f) => f.from), ...own] })}
                  problems={problems}
                  readOnly={readOnly}
                />
              </div>
              {!object.box && (
                <BoxLabelsEditor
                  object={object}
                  onChange={(box_labels) => setObject(i, { box_labels: box_labels.length ? box_labels : undefined })}
                  problems={under(problems, `objects[${i}].box_labels`)}
                  readOnly={false} // até num padrão: labels novas aparecem com tarefas novas
                />
              )}
            </article>
          )
        })}
        {!readOnly && <button className="pe-add" onClick={addObject} type="button">Adicionar tipo de objeto</button>}
      </section>

      {problems.length > 0 && (
        <div className="pe-problems" id={`${uid}-problems`} role="alert">
          <strong>{problems.length === 1 ? 'Falta corrigir 1 coisa' : `Faltam corrigir ${problems.length} coisas`}</strong>
          <ul>
            {problems.map((p) => {
              const target = p.path.match(/^(objects\[\d+\]\.)?fields\[\d+\]/)?.[0] ?? p.path.match(/^objects\[\d+\]/)?.[0]
              return (
                <li key={p.path + p.msg}>
                  {target ? (
                    <button className="set-link" onClick={() => document.getElementById(pathId(target))?.scrollIntoView({ block: 'center', behavior: 'smooth' })} type="button">{p.msg}</button>
                  ) : p.msg}
                  <code>{p.path}</code>
                </li>
              )
            })}
          </ul>
        </div>
      )}

      <div className={dirty ? 'set-savebar set-savebar--dirty' : 'set-savebar'} role="status">
        <span>{saving ? 'Salvando…' : dirty ? (isNew ? 'Perfil ainda não criado.' : 'Alterações não salvas.') : readOnly ? 'Perfil padrão.' : isNew ? 'Preencha o nome e os campos.' : 'Tudo salvo.'}</span>
        <div>
          <button className="button button--secondary button--sm" onClick={() => void close()} type="button">{dirty ? 'Descartar' : 'Voltar à lista'}</button>
          <button aria-busy={saving || undefined} className="button button--primary button--sm" disabled={saving} onClick={() => void save()} type="button">
            {saving ? 'Salvando…' : isNew ? 'Criar perfil' : 'Salvar perfil'}
          </button>
        </div>
      </div>
    </div>
  )
}
