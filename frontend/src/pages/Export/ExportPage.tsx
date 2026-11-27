import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import {
  createExportProfile,
  deleteExportProfile,
  downloadExport,
  getExportFolderFields,
  listAnnotationProfiles,
  listExportProfiles,
  listPackageProfiles,
  listTasks,
  previewExport,
  updateExportProfile,
  type AnnotationTask,
  type ExportFilters,
  type ExportPreview,
  type ExportProfile,
  type ExportFieldsInfo,
} from '../../services/api'
import type { AnnotationProfile, ProfileField } from '../../types/annotation'
import { FileNameTemplate } from './FileNameTemplate'
import { FolderLevels } from './FolderLevels'
import { Link } from 'react-router-dom'
import { useToastState } from '../../hooks/useToast'
import { confirmDialog } from '../../services/confirm'
import { useIsAdmin } from '../../services/user'
import { PageLoader } from '../../components/PageLoader/PageLoader'
import { HelpTip } from '../../components/HelpTip/HelpTip'
import './Export.css'

const fmt = (n: number) => n.toLocaleString('pt-BR')

const FORMATS = [
  ['coco', 'RF-DETR (COCO)', 'train/, valid/, test/ com _annotations.coco.json (layout do Roboflow)'],
  ['yolo', 'YOLO (Ultralytics)', 'images/, labels/ e data.yaml'],
  ['voc', 'Pascal VOC', 'um .xml por imagem, ao lado dela'],
  ['images', 'Só imagens', 'as imagens e um anotacoes.csv, sem caixas'],
] as const

const FORMAT_NAMES: Record<ExportProfile['format'], string> = {
  coco: 'RF-DETR (COCO)', yolo: 'YOLO', voc: 'Pascal VOC', images: 'Só imagens',
}

// Formatos com pastas pelos metadados (COCO e YOLO precisam de train/valid/test).
const withFolders = (format: ExportProfile['format']) => format === 'voc' || format === 'images'

// A divisão importa? (por divisão, ou com um nível "split" nas pastas)
function usesSplit(profile: ExportProfile): boolean {
  return profile.layout !== 'folders' || (profile.folders ?? []).some((level) => level.field === 'split')
}

const SPLIT_LABELS = { train: 'Treino', valid: 'Validação', test: 'Teste' } as const

const DEFAULT_ANNOTATION_PROFILE = 'veiculos'
const label = (item: { key: string; label?: string }) => item.label || item.key

// Um filtro: lista vira <select>, sim/não também; o resto é texto livre.
// Data e hora não filtra.
function FilterField({ field, value, onChange }: { field: ProfileField; value: string; onChange: (value: string) => void }) {
  if (field.type === 'datetime') return null
  const options: Array<[string, string]> | null = field.type === 'choice' || field.type === 'multi'
    ? (field.options ?? []).map((o) => [o.value, o.label || o.value])
    : field.type === 'bool' ? [['true', 'Sim'], ['false', 'Não']] : null
  return (
    <label className="form-field">
      <span>{label(field)}</span>
      {options ? (
        <select onChange={(event) => onChange(event.target.value)} value={value}>
          <option value="">Qualquer</option>
          {options.map(([v, name]) => <option key={v} value={v}>{name}</option>)}
          {value && !options.some(([v]) => v === value) && <option value={value}>{value} (não existe no perfil)</option>}
        </select>
      ) : (
        <input
          inputMode={field.type === 'number' ? 'decimal' : undefined}
          onChange={(event) => onChange(event.target.value.trim())}
          placeholder="Qualquer"
          value={value}
        />
      )}
    </label>
  )
}

type LabelRow = { label: string; count: number; checked: boolean; rename: string }

function newProfile(): ExportProfile {
  return { name: '', format: 'coco', split: { train: 70, valid: 15, test: 15 }, filters: {}, labels: {} }
}

// Perfil sem tarefa (os padrões) e só uma tarefa no sistema: já vem escolhida.
function withDefaultTask(profile: ExportProfile, tasks: AnnotationTask[]): ExportProfile {
  if (!profile.tasks?.length && tasks.length === 1) return { ...profile, tasks: [tasks[0].name] }
  return profile
}

// As labels de uma tarefa, com quantas caixas cada (as da lista da tarefa e
// as que têm caixas).
function taskLabels(task: AnnotationTask | undefined): Record<string, number> {
  if (!task) return {}
  const out: Record<string, number> = {}
  for (const label of task.labels) out[label] = 0
  return { ...out, ...(task.stats?.labels ?? {}) }
}

// As labels de várias tarefas, somadas: a mesma label em duas tarefas vira
// uma classe só (renomeie para separar).
function tasksLabels(all: AnnotationTask[], names: string[] | undefined): Record<string, number> {
  const out: Record<string, number> = {}
  for (const name of names ?? []) {
    for (const [label, n] of Object.entries(taskLabels(all.find((task) => task.name === name)))) {
      out[label] = (out[label] ?? 0) + n
    }
  }
  return out
}

// Linhas da tabela de labels a partir do perfil: cada label da tarefa, se
// entra e com que nome. Labels do perfil que não existem na tarefa também
// aparecem (para não sumirem ao salvar).
function rowsFrom(profile: ExportProfile, existing: Record<string, number>): LabelRow[] {
  const rename = profile.labels.rename ?? {}
  const include = profile.labels.include ?? []
  const names = new Set([...Object.keys(existing), ...Object.keys(rename)])
  return [...names].sort((a, b) => a.localeCompare(b)).map((label) => {
    const final = rename[label] ?? label
    return {
      label,
      count: existing[label] ?? 0,
      checked: include.length === 0 || include.includes(final),
      rename: rename[label] ?? '',
    }
  })
}

// O caminho de volta: tabela -> regras do perfil.
function labelsFrom(rows: LabelRow[], previousInclude: string[] | undefined): ExportProfile['labels'] {
  const rename: Record<string, string> = {}
  const finals: string[] = []
  for (const row of rows) {
    const final = row.rename.trim() || row.label
    if (row.rename.trim() && row.rename.trim() !== row.label) rename[row.label] = row.rename.trim()
    if (row.checked && !finals.includes(final)) finals.push(final)
  }
  const allChecked = rows.every((row) => row.checked)
  // Mantém a ordem que o perfil já tinha (ela define os ids das classes).
  const ordered = previousInclude
    ? [...previousInclude.filter((name) => finals.includes(name)), ...finals.filter((name) => !previousInclude.includes(name)).sort()]
    : [...finals].sort()
  return {
    ...(Object.keys(rename).length > 0 ? { rename } : {}),
    ...(allChecked && !previousInclude?.length ? {} : { include: ordered }),
  }
}

// O cabeçalho de cada seção: o título, uma linha do que ela faz e, se
// precisar, os detalhes no (?). Mesmo formato em toda seção.
function SectionHead({ title, hint, help }: { title: string; hint?: ReactNode; help?: ReactNode }) {
  return (
    <div className="export-section-head">
      <h3>{title}{help && <HelpTip label={title}>{help}</HelpTip>}</h3>
      {hint && <p>{hint}</p>}
    </div>
  )
}

export function ExportPage() {
  const admin = useIsAdmin() // salvar e apagar perfis: só admin
  const [profiles, setProfiles] = useState<ExportProfile[]>([])
  const [packageProfiles, setPackageProfiles] = useState<Record<string, string>>({}) // pacote -> perfil de anotação
  const [tasks, setTasks] = useState<AnnotationTask[]>([])
  const [annotationProfiles, setAnnotationProfiles] = useState<AnnotationProfile[]>([])
  const [folderFields, setFolderFields] = useState<ExportFieldsInfo>({ annotation_profile: '', fields: [], unknown: 'Desconhecido', name_fields: [], legacy_template: '' })
  const [selected, setSelected] = useState<string | null>(null) // nome do perfil salvo/padrão aberto
  const [draft, setDraft] = useState<ExportProfile>(newProfile())
  const [rows, setRows] = useState<LabelRow[]>([])
  const [preview, setPreview] = useState<ExportPreview | null>(null)
  const [busy, setBusy] = useState<'' | 'preview' | 'save' | 'delete'>('')
  const [, setError] = useToastState('error')
  const [, setMessage] = useToastState('success')
  const [loaded, setLoaded] = useState(false) // a primeira carga (perfis, pacotes, tarefas)

  useEffect(() => {
    let active = true
    Promise.all([listExportProfiles(), listPackageProfiles(), listTasks(), listAnnotationProfiles()])
      .then(([profileList, pkgProfiles, taskList, annProfiles]) => {
        if (!active) return
        setProfiles(profileList)
        setPackageProfiles(pkgProfiles)
        setTasks(taskList)
        setAnnotationProfiles(annProfiles)
        const first = profileList[0]
        if (first) {
          const next = withDefaultTask(structuredClone(first), taskList)
          setSelected(first.name)
          setDraft(next)
          setRows(rowsFrom(next, tasksLabels(taskList, next.tasks)))
        }
      })
      .catch((requestError: unknown) => { if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar o export.') })
      .finally(() => { if (active) setLoaded(true) })
    return () => { active = false }
  }, [setError])

  // Os campos de pasta e de nome do perfil de anotação escolhido.
  const annotationName = draft.annotation_profile || DEFAULT_ANNOTATION_PROFILE
  useEffect(() => {
    let active = true
    getExportFolderFields(annotationName)
      .then((fields) => { if (active) setFolderFields(fields) })
      .catch((requestError: unknown) => { if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os campos do perfil de anotação.') })
    return () => { active = false }
  }, [annotationName, setError])
  const annotationProfile = annotationProfiles.find((profile) => profile.name === annotationName)
  const packages = Object.keys(packageProfiles).sort((a, b) => a.localeCompare(b))

  const original = profiles.find((profile) => profile.name === selected)
  const isBuiltin = Boolean(original?.builtin)
  const splitSum = draft.split.train + draft.split.valid + draft.split.test
  const noBoxes = draft.format === 'images' // só imagens: sem tarefa, sem labels
  const isFolders = withFolders(draft.format) && draft.layout === 'folders'
  const splitMatters = usesSplit(draft)
  const fileNameOk = !draft.file_name || /\{(sha1|name)\}/.test(draft.file_name)
  const foldersOk = (!isFolders || (draft.folders ?? []).length > 0) && fileNameOk
  const hasTasks = (draft.tasks?.length ?? 0) > 0
  const canRun = splitSum === 100 && (hasTasks || noBoxes) && foldersOk

  // O perfil como está na tela (com as labels da tabela).
  const current = useMemo<ExportProfile>(
    () => noBoxes
      ? { ...draft, tasks: undefined, all_tasks: undefined, labels: {} }
      : { ...draft, labels: labelsFrom(rows, original?.labels.include) },
    [draft, rows, original, noBoxes],
  )
  const dirty = useMemo(() => !original || JSON.stringify(original) !== JSON.stringify({ ...original, ...current }), [original, current])

  const labelsOf = (names: string[] | undefined) => tasksLabels(tasks, names)

  function open(profile: ExportProfile | null) {
    const next = withDefaultTask(profile ? structuredClone(profile) : newProfile(), tasks)
    setSelected(profile?.name ?? null)
    setDraft(next)
    setRows(rowsFrom(next, labelsOf(next.tasks)))
    setPreview(null)
    setError('')
    setMessage('')
  }

  function change(patch: Partial<ExportProfile>) {
    setDraft((currentDraft) => ({ ...currentDraft, ...patch }))
    setPreview(null)
  }

  // Pôr/tirar uma tarefa troca a tabela de labels (cada tarefa tem as suas).
  // Ao passar a ter duas, "só as anotadas em todas" já vem marcado.
  function toggleTask(name: string) {
    const current = draft.tasks ?? []
    const list = current.includes(name) ? current.filter((t) => t !== name) : [...current, name]
    const next: ExportProfile = { ...draft, tasks: list.length > 0 ? list : undefined }
    if (current.length < 2 && list.length >= 2 && draft.all_tasks === undefined) next.all_tasks = true
    if (list.length < 2) next.all_tasks = undefined
    setDraft(next)
    setRows(rowsFrom(next, labelsOf(next.tasks)))
    setPreview(null)
  }

  // objectType vazio: campo da imagem; senão, campo do objeto desse tipo.
  function changeFilter(objectType: string, key: string, value: string) {
    setDraft((currentDraft) => {
      const filters: ExportFilters = { ...currentDraft.filters }
      if (!objectType) {
        const fields = { ...(filters.fields ?? {}) }
        if (value) fields[key] = value
        else delete fields[key]
        filters.fields = Object.keys(fields).length > 0 ? fields : undefined
      } else {
        const objects = { ...(filters.objects ?? {}) }
        const fields = { ...(objects[objectType] ?? {}) }
        if (value) fields[key] = value
        else delete fields[key]
        if (Object.keys(fields).length > 0) objects[objectType] = fields
        else delete objects[objectType]
        filters.objects = Object.keys(objects).length > 0 ? objects : undefined
      }
      return { ...currentDraft, filters }
    })
    setPreview(null)
  }

  // Trocar o perfil de anotação troca os campos: os filtros e as pastas
  // começam de novo (o modelo de nome fica, e avisa do que o perfil não tem).
  function changeAnnotationProfile(name: string) {
    change({
      annotation_profile: name === DEFAULT_ANNOTATION_PROFILE ? undefined : name,
      filters: {},
      ...(draft.layout === 'folders' ? { folders: [] } : {}),
    })
  }

  function changeRow(index: number, patch: Partial<LabelRow>) {
    setRows((currentRows) => currentRows.map((row, i) => (i === index ? { ...row, ...patch } : row)))
    setPreview(null)
  }

  function togglePackage(name: string) {
    const list = draft.packages ?? []
    change({ packages: list.includes(name) ? list.filter((item) => item !== name) : [...list, name] })
  }

  async function runPreview() {
    setBusy('preview')
    setError('')
    setMessage('')
    try {
      setPreview(await previewExport(current))
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível gerar a prévia.')
    } finally {
      setBusy('')
    }
  }

  async function save() {
    setBusy('save')
    setError('')
    setMessage('')
    try {
      const name = current.name.trim().replace(/\s+/g, '-')
      const profile = { ...current, name }
      const saved = selected && !isBuiltin && selected === name
        ? await updateExportProfile(profile)
        : await createExportProfile(profile)
      const list = await listExportProfiles()
      setProfiles(list)
      setSelected(saved.name)
      setDraft(structuredClone(saved))
      setRows(rowsFrom(saved, labelsOf(saved.tasks)))
      setMessage(`Perfil "${saved.name}" salvo.`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar o perfil.')
    } finally {
      setBusy('')
    }
  }

  async function remove() {
    if (!selected || isBuiltin) return
    if (!(await confirmDialog({
      title: 'Apagar perfil de export?',
      message: `O perfil "${selected}" será apagado.`,
      confirmLabel: 'Apagar perfil',
      tone: 'danger',
    }))) return
    setBusy('delete')
    try {
      await deleteExportProfile(selected)
      const list = await listExportProfiles()
      setProfiles(list)
      open(list[0] ?? null)
      setMessage(`Perfil "${selected}" apagado.`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar o perfil.')
    } finally {
      setBusy('')
    }
  }

  const sameNameAsBuiltin = profiles.some((profile) => profile.builtin && profile.name === draft.name.trim())
  const canSave = draft.name.trim() !== '' && splitSum === 100 && foldersOk && !sameNameAsBuiltin && (dirty || !selected)
  const firstFolderField = folderFields.fields.find((field) => field.folder && !field.alias && field.group !== 'file')?.field ?? 'package'
  // Pacotes de outro perfil de anotação entram, mas sem os campos (não
  // passam nos filtros; vão para "Desconhecido").
  const otherProfile = (name: string) => Boolean(packageProfiles[name]) && packageProfiles[name] !== annotationName

  if (!loaded) return <section className="export-page"><PageLoader label="Carregando o export" /></section>

  return (
    <section className="export-page">
      <div className="page-heading">
        <div>
          <h1>Exportar</h1>
          <p>Datasets de treino com as imagens verificadas: com as caixas do CVAT ou só as imagens.</p>
        </div>
      </div>

      <div className="export-layout">
        <aside className="export-profiles">
          <span className="export-profiles__title">Perfis</span>
          {profiles.map((profile) => (
            <button
              className={profile.name === selected ? 'export-profile export-profile--active' : 'export-profile'}
              key={profile.name}
              onClick={() => open(profile)}
              type="button"
            >
              <strong>{profile.name}</strong>
              <span>{profile.builtin ? 'padrão' : FORMAT_NAMES[profile.format] ?? profile.format}</span>
            </button>
          ))}
          <button className="button button--secondary export-profiles__new" onClick={() => open(null)} type="button">+ Novo perfil</button>
        </aside>

        <div className="export-editor">
          <div className="create-card settings-card">
            <h2>{selected ? (isBuiltin ? `${selected} (padrão)` : selected) : 'Novo perfil'}</h2>
            {isBuiltin && (
              <p className="profile-name-hint">Perfil padrão: para guardar mudanças, salve com outro nome.</p>
            )}

            <div className="profile-form-grid">
              <label className="form-field">
                <span>Nome</span>
                <input onChange={(event) => change({ name: event.target.value })} placeholder="Ex.: yolo-placas-noite" value={draft.name} />
                {sameNameAsBuiltin && dirty && <small className="profile-name-hint">Para salvar, use um nome diferente de "{draft.name}".</small>}
              </label>
              <label className="form-field">
                <span>Descrição (opcional)</span>
                <input onChange={(event) => change({ description: event.target.value })} value={draft.description ?? ''} />
              </label>
            </div>

            {!noBoxes && <>
            <SectionHead
              help={(
                <ul>
                  <li>Várias juntam as caixas num dataset só (ex.: para-brisa + placa).</li>
                  {isBuiltin && <li>Os perfis padrão servem para qualquer tarefa: escolha na hora.</li>}
                </ul>
              )}
              hint="As caixas de quais tarefas entram no dataset."
              title="Tarefa"
            />
            {tasks.length === 0 ? (
              <p className="form-error">
                Nenhuma tarefa ainda: crie uma em <Link to="/tarefas">Tarefas</Link> e importe as caixas do CVAT.
              </p>
            ) : (
              <>
                <div className="export-packages">
                  {tasks.map((task) => (
                    <label className="export-check" key={task.name}>
                      <input checked={draft.tasks?.includes(task.name) ?? false} onChange={() => toggleTask(task.name)} type="checkbox" />
                      {task.name} <small className="export-check__profile">({fmt(task.stats?.verified ?? 0)} verificadas)</small>
                    </label>
                  ))}
                  {(draft.tasks ?? []).filter((name) => !tasks.some((task) => task.name === name)).map((name) => (
                    <label className="export-check" key={name}>
                      <input checked onChange={() => toggleTask(name)} type="checkbox" />
                      {name} <small className="form-error">(não existe mais)</small>
                    </label>
                  ))}
                </div>
                {(draft.tasks?.length ?? 0) > 1 && (
                  <>
                    <label className="export-check export-all-tasks">
                      <input checked={Boolean(draft.all_tasks)} onChange={(event) => change({ all_tasks: event.target.checked || undefined })} type="checkbox" />
                      Só as imagens anotadas em todas as tarefas
                    </label>
                    <p className={draft.all_tasks ? 'profile-name-hint' : 'profile-name-hint export-warning'}>
                      {draft.all_tasks
                        ? 'Imagens anotadas só em parte das tarefas ficam de fora.'
                        : 'Atenção: imagem anotada só numa tarefa entra sem as caixas das outras (o modelo aprende que é fundo).'}
                    </p>
                  </>
                )}
              </>
            )}
            </>}

            <SectionHead
              help={(
                <ul>
                  <li>Use o perfil dos pacotes que vão no dataset.</li>
                  <li>Imagens de outro perfil entram sem esses campos: não passam nos filtros e vão para "Desconhecido" nas pastas.</li>
                </ul>
              )}
              hint="Dá os campos dos filtros, das pastas e do nome dos arquivos."
              title="Perfil de anotação"
            />
            <div className="profile-form-grid">
              <label className="form-field">
                <span>Perfil</span>
                <select onChange={(event) => changeAnnotationProfile(event.target.value)} value={annotationName}>
                  {annotationProfiles.map((profile) => <option key={profile.name} value={profile.name}>{profile.name}</option>)}
                  {!annotationProfile && annotationProfiles.length > 0 && <option value={annotationName}>{annotationName} (não existe mais)</option>}
                </select>
              </label>
            </div>

            <SectionHead hint="Como o dataset sai dentro do zip." title="Formato" />
            <div className="export-formats">
              {FORMATS.map(([value, label, hint]) => (
                <label className={draft.format === value ? 'export-format export-format--active' : 'export-format'} key={value}>
                  <input
                    checked={draft.format === value}
                    name="format"
                    onChange={() => change(withFolders(value) ? { format: value } : { format: value, layout: 'split', folders: undefined })}
                    type="radio"
                  />
                  <strong>{label}</strong>
                  <span>{hint}</span>
                </label>
              ))}
            </div>

            {withFolders(draft.format) && (
              <>
                <SectionHead hint="Como as imagens se organizam no zip." title="Pastas de saída" />
                <div className="export-formats">
                  <label className={!isFolders ? 'export-format export-format--active' : 'export-format'}>
                    <input checked={!isFolders} name="layout" onChange={() => change({ layout: 'split', folders: undefined })} type="radio" />
                    <strong>Por divisão</strong>
                    <span>{noBoxes ? 'train/, valid/ e test/ (com tudo em treino, as imagens ficam na raiz do zip)' : 'train/, valid/ e test/, cada imagem com o seu .xml'}</span>
                  </label>
                  <label className={isFolders ? 'export-format export-format--active' : 'export-format'}>
                    <input
                      checked={isFolders}
                      name="layout"
                      onChange={() => change({ layout: 'folders', folders: draft.folders?.length ? draft.folders : [{ field: firstFolderField }], split: { train: 100, valid: 0, test: 0 } })}
                      type="radio"
                    />
                    <strong>Por pastas (metadados)</strong>
                    <span>uma árvore montada com os dados de cada imagem, ex.: Car/01-640x480/Br/Traseiro/Noturno</span>
                  </label>
                </div>
                {isFolders && (
                  <FolderLevels
                    fields={folderFields.fields}
                    levels={draft.folders ?? []}
                    onChange={(folders) => {
                      // Pôs o nível "Divisão" com tudo em treino: começa em 70/15/15.
                      const addedSplit = folders.some((level) => level.field === 'split') && !usesSplit(draft)
                      change(addedSplit && draft.split.train === 100 ? { folders, split: { train: 70, valid: 15, test: 15 } } : { folders })
                    }}
                    unknownDefault={folderFields.unknown}
                  />
                )}
              </>
            )}

            {splitMatters && <>
            <SectionHead
              help={(
                <ul>
                  <li>Fixa por imagem: re-exportar com imagens novas não muda o grupo das antigas.</li>
                  {draft.format === 'yolo' && <li>No YOLO, a validação não pode ser 0.</li>}
                  {isFolders && <li>Com o nível "Divisão" nas pastas, train/valid/test viram pastas na posição dele.</li>}
                </ul>
              )}
              hint="Quanto vai para treino, validação e teste."
              title="Divisão"
            />
            <div className="export-split">
              {(['train', 'valid', 'test'] as const).map((key) => (
                <label className="form-field" key={key}>
                  <span>{SPLIT_LABELS[key]} (%)</span>
                  <input
                    max={100}
                    min={0}
                    onChange={(event) => change({ split: { ...draft.split, [key]: Math.max(0, Number.parseInt(event.target.value, 10) || 0) } })}
                    type="number"
                    value={draft.split[key]}
                  />
                </label>
              ))}
              <span className={splitSum === 100 ? 'export-split__sum' : 'export-split__sum export-split__sum--bad'}>
                Soma: {splitSum}%
              </span>
            </div>
            </>}

            <SectionHead
              help={(
                <ul>
                  <li>Campo de lista com código sai com o código (ex.: direção F/T).</li>
                  <li>Campo sem valor vira "_" na largura dele.</li>
                  <li>Campo de objeto: o do primeiro objeto do tipo; de uma lista, o primeiro valor.</li>
                  <li>A extensão é a do arquivo. O exemplo aparece na prévia.</li>
                </ul>
              )}
              hint="Como cada imagem se chama no zip."
              title="Nome dos arquivos"
            />
            <FileNameTemplate
              key={selected ?? 'novo'}
              fields={folderFields.fields}
              legacy={folderFields.legacy_template}
              onChange={(fileName) => change({ file_name: fileName || undefined })}
              value={draft.file_name ?? ''}
            />

            <SectionHead hint="De quais pacotes vêm as imagens." title="Pacotes" />
            <div className="export-packages">
              <label className="export-check">
                <input checked={!draft.packages?.length} onChange={() => change({ packages: [] })} type="checkbox" />
                Todos
              </label>
              {packages.map((name) => (
                <label className="export-check" key={name} title={otherProfile(name) ? `perfil de anotação ${packageProfiles[name]}` : undefined}>
                  <input checked={draft.packages?.includes(name) ?? false} onChange={() => togglePackage(name)} type="checkbox" />
                  {name}
                  {otherProfile(name) && <small className="export-check__profile">({packageProfiles[name]})</small>}
                </label>
              ))}
            </div>

            <SectionHead hint="Só entram as imagens verificadas com estes valores." title="Filtros" />
            {!annotationProfile ? (
              <p className="profile-name-hint">Carregando os campos do perfil de anotação…</p>
            ) : (
              <>
                <div className="profile-form-grid export-filters">
                  {annotationProfile.fields.map((field) => (
                    <FilterField
                      field={field}
                      key={field.key}
                      onChange={(value) => changeFilter('', field.key, value)}
                      value={draft.filters.fields?.[field.key] ?? ''}
                    />
                  ))}
                </div>
                {annotationProfile.objects.filter((object) => !object.box).map((object) => (
                  <div key={object.key}>
                    <h4 className="export-filters__object">{label(object)} <small>(todos no mesmo objeto)</small></h4>
                    <div className="profile-form-grid export-filters">
                      {object.fields.map((field) => (
                        <FilterField
                          field={field}
                          key={field.key}
                          onChange={(value) => changeFilter(object.key, field.key, value)}
                          value={draft.filters.objects?.[object.key]?.[field.key] ?? ''}
                        />
                      ))}
                    </div>
                  </div>
                ))}
              </>
            )}

            {!noBoxes && <>
            <SectionHead
              help={(
                <ul>
                  <li>Mesmo nome em duas labels junta as duas numa classe (ex.: "Placa" e "placa").</li>
                  <li>A mesma label em duas tarefas vira uma classe só: renomeie para separar.</li>
                </ul>
              )}
              hint="Quais labels entram e com que nome no dataset."
              title="Labels"
            />
            {!hasTasks ? (
              <p className="profile-name-hint">Escolha a tarefa para ver as labels dela.</p>
            ) : rows.length === 0 ? (
              <p className="profile-name-hint">{(draft.tasks?.length ?? 0) > 1 ? 'Estas tarefas ainda não têm' : 'Esta tarefa ainda não tem'} labels nem caixas.</p>
            ) : (
              <table className="export-labels">
                <thead>
                  <tr><th>Entra</th><th>Label da tarefa</th><th>Caixas</th><th>Nome no dataset</th></tr>
                </thead>
                <tbody>
                  {rows.map((row, index) => (
                    <tr className={row.checked ? '' : 'export-labels__off'} key={row.label}>
                      <td><input checked={row.checked} onChange={(event) => changeRow(index, { checked: event.target.checked })} type="checkbox" /></td>
                      <td>{row.label}</td>
                      <td>{fmt(row.count)}</td>
                      <td>
                        <input
                          onChange={(event) => changeRow(index, { rename: event.target.value })}
                          placeholder={row.label}
                          value={row.rename}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            </>}

            <div className="export-actions">
              <button
                className="button button--secondary"
                disabled={busy !== '' || !canRun}
                onClick={() => void runPreview()}
                title={!hasTasks && !noBoxes ? 'Escolha a tarefa antes' : !foldersOk ? 'Escolha pelo menos um nível de pasta' : undefined}
                type="button"
              >
                {busy === 'preview' ? 'Calculando...' : 'Prévia'}
              </button>
              <button
                className="button button--primary"
                disabled={!preview || preview.total === 0}
                onClick={() => {
                  downloadExport(current)
                  setMessage('O download começa em instantes (o zip é gerado enquanto baixa).')
                }}
                title={!preview ? 'Faça a prévia antes' : undefined}
                type="button"
              >
                Baixar zip
              </button>
              <span className="export-actions__spacer" />
              {admin && selected && !isBuiltin && (
                <button className="button button--danger" disabled={busy !== ''} onClick={() => void remove()} type="button">Apagar perfil</button>
              )}
              {admin && (
                <button className="button button--secondary" disabled={busy !== '' || !canSave} onClick={() => void save()} type="button">
                  {busy === 'save' ? 'Salvando...' : selected && !isBuiltin && selected === draft.name.trim() ? 'Salvar alterações' : 'Salvar como novo perfil'}
                </button>
              )}
            </div>
          </div>

          {preview && (
            <div className="create-card settings-card export-preview">
              <h2>Prévia</h2>
              {preview.total === 0 ? (
                <p className="form-error">
                  {preview.format === 'images' ? 'Nenhuma imagem verificada passa nestes filtros.' : <>Nenhuma imagem verificada e anotada {preview.tasks.length > 1 ? `${preview.all_tasks ? 'em todas as tarefas' : 'nas tarefas'} "${preview.task}"` : `na tarefa "${preview.task}"`} passa nestes filtros.</>}
                </p>
              ) : (
                <>
                  <div className="import-report__stats">
                    <span><strong>{fmt(preview.total)}</strong> imagens</span>
                    {preview.negatives > 0 && <span><strong>{fmt(preview.negatives)}</strong> sem objetos</span>}
                    {preview.layout === 'folders'
                      ? <span><strong>{fmt(Object.keys(preview.folders ?? {}).length)}</strong> pastas</span>
                      : (['train', 'valid', 'test'] as const).map((key) => (
                        <span key={key}><strong>{fmt(preview.images[key] ?? 0)}</strong> {SPLIT_LABELS[key].toLowerCase()}</span>
                      ))}
                  </div>
                  {preview.examples?.length > 0 && (
                    <div className="export-examples">
                      <span>Exemplos de nome:</span>
                      {preview.examples.map((name) => <code key={name}>{name}</code>)}
                    </div>
                  )}
                  {preview.layout === 'folders' && (
                    <table className="export-labels export-folders">
                      <thead><tr><th>Pasta</th><th>Imagens</th></tr></thead>
                      <tbody>
                        {Object.entries(preview.folders ?? {}).sort(([a], [b]) => a.localeCompare(b)).map(([dir, n]) => (
                          <tr key={dir}><td><code>{dir}/</code></td><td>{fmt(n)}</td></tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                  {preview.format !== 'images' && <table className="export-labels">
                    <thead><tr>{preview.format !== 'voc' && <th>Id</th>}<th>Classe</th><th>Caixas</th></tr></thead>
                    <tbody>
                      {preview.classes.map((name, i) => (
                        <tr key={name}>
                          {preview.format !== 'voc' && <td>{preview.format === 'yolo' ? i : i + 1}</td>}
                          <td>{name}</td>
                          <td>{fmt(preview.boxes[name] ?? 0)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>}
                  {preview.format === 'images' && (
                    <p className="profile-name-hint">O zip leva também o anotacoes.csv (abre no Excel).</p>
                  )}
                </>
              )}
              {(preview.skipped.not_annotated > 0 || preview.skipped.missing_tasks > 0 || preview.skipped.without_labels > 0) && (
                <p className="profile-name-hint">
                  Ficaram de fora: {fmt(preview.skipped.not_annotated)} verificada(s) que não estão {preview.tasks.length > 1 ? `em nenhuma das tarefas "${preview.task}"` : `na tarefa "${preview.task}"`}
                  {preview.skipped.missing_tasks > 0 && `, ${fmt(preview.skipped.missing_tasks)} anotada(s) em só parte das tarefas`}
                  {preview.skipped.without_labels > 0 && ` e ${fmt(preview.skipped.without_labels)} sem nenhuma das labels escolhidas`}.
                  Imagens pendentes não entram.
                </p>
              )}
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
