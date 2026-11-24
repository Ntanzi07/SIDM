import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties, FormEvent, KeyboardEvent, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  createTask,
  deleteTask,
  getTask,
  imagesZipUrl,
  listPackageSummaries,
  listTasks,
  updateTask,
  type AnnotationTask,
  type PackageSummary,
} from '../../services/api'
import { autoColor, colorOf } from '../../utils/boxes'
import { setTaskColors, useTaskColors } from '../../utils/taskColors'
import { BoxImportModal } from '../Packages/BoxImportModal'
import { GridPicker, Menu, SearchField, SearchIcon } from '../../components/Toolbar/Toolbar'
import { savedColumns, saveColumns, type Columns, type MenuOption } from '../../components/Toolbar/columns'
import { useIsAdmin } from '../../services/user'
import '../Packages/Packages.css'
import { PageLoader } from '../../components/PageLoader/PageLoader'
import './Tasks.css'

const fmt = (n: number) => n.toLocaleString('pt-BR')
const pct = (n: number, of: number) => (of > 0 ? `${Math.round((n / of) * 100)}%` : '0%')
const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/
const NAME_HINT = 'Só letras minúsculas, números, - e _ (sem espaço).'

// "a, b\nc" -> ["a", "b", "c"] (sem vazias nem repetidas, na ordem).
function splitLabels(text: string): string[] {
  const out: string[] = []
  for (const part of text.split(/[\n,;]/)) {
    const label = part.trim()
    if (label && !out.includes(label)) out.push(label)
  }
  return out
}

// Tarefas: cada uma é um conjunto de labels (as classes das caixas) e as
// caixas rotuladas nelas. Sem nome na rota: a grade de tarefas; com nome: a
// tarefa aberta.
export function TasksPage() {
  const { name } = useParams()
  return name ? <TaskDetail key={name} name={name} /> : <TaskGrid />
}

// ── A grade ─────────────────────────────────────────────────────────────
const TASK_COLUMNS = [4, 3, 2, 1] as const satisfies readonly Columns[]
const TASK_COLUMNS_KEY = 'sidm.tasks.columns'

type SortBy = 'updated' | 'name' | 'images' | 'boxes'

const SORTS: MenuOption[] = [
  { value: 'updated', label: 'última modificação' },
  { value: 'name', label: 'nome' },
  { value: 'images', label: 'mais imagens' },
  { value: 'boxes', label: 'mais caixas' },
]

const time = (iso?: string) => (iso ? new Date(iso).getTime() || 0 : 0)

function TaskGrid() {
  const admin = useIsAdmin() // criar tarefa: só admin
  const [tasks, setTasks] = useState<AnnotationTask[] | null>(null)
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [query, setQuery] = useState('')
  const [sortBy, setSortBy] = useState<SortBy>('updated')
  const [columns, setColumns] = useState<Columns>(() => savedColumns(TASK_COLUMNS_KEY, 3, TASK_COLUMNS))
  useTaskColors()

  // busca no nome, na descrição e nas labels
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = (tasks ?? []).filter((task) => !q
      || task.name.toLowerCase().includes(q)
      || task.description.toLowerCase().includes(q)
      || task.labels.some((label) => label.toLowerCase().includes(q)))
    const by: Record<SortBy, (a: AnnotationTask, b: AnnotationTask) => number> = {
      updated: (a, b) => time(b.updated_at) - time(a.updated_at),
      name: (a, b) => a.name.localeCompare(b.name),
      images: (a, b) => (b.stats?.images ?? 0) - (a.stats?.images ?? 0),
      boxes: (a, b) => (b.stats?.boxes ?? 0) - (a.stats?.boxes ?? 0),
    }
    return [...list].sort((a, b) => by[sortBy](a, b) || a.name.localeCompare(b.name))
  }, [tasks, query, sortBy])

  function chooseColumns(value: Columns) {
    setColumns(value)
    saveColumns(TASK_COLUMNS_KEY, value)
  }

  useEffect(() => {
    let active = true
    listTasks()
      .then((list) => { if (active) setTasks(list) })
      .catch((requestError: unknown) => {
        if (!active) return
        setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as tarefas.')
        setTasks([])
      })
    return () => { active = false }
  }, [])

  return (
    <section className="tasks-page">
      <div className="page-heading">
        <div>
          <h1>Tarefas</h1>
          <p>Cada tarefa tem as suas labels e as suas caixas (ex.: capacete, letras da placa). A mesma imagem pode estar em várias.</p>
        </div>
        {admin && <button className="button button--primary" onClick={() => setCreating(true)} type="button">+ Nova tarefa</button>}
      </div>
      {error && <p className="form-error">{error}</p>}
      {tasks === null && <PageLoader label="Carregando as tarefas" />}
      {tasks?.length === 0 && !error && (
        <div className="tasks-empty">
          <p>Nenhuma tarefa ainda.</p>
          <p className="tasks-hint">Crie uma com o nome e as labels que você vai rotular.</p>
        </div>
      )}
      {tasks && tasks.length > 0 && (
        <div className="package-toolbar">
          <SearchField id="task-search" onChange={setQuery} placeholder="Pesquisar tarefa, descrição ou label" value={query} />
          <div className="package-toolbar__actions">
            <span className="package-toolbar__count">{shown.length} de {tasks.length} tarefa(s)</span>
            <Menu label="Ordenar por" onChange={(value) => setSortBy(value as SortBy)} options={SORTS} value={sortBy} />
            <GridPicker onChange={chooseColumns} options={TASK_COLUMNS} value={columns} />
          </div>
        </div>
      )}
      {tasks && tasks.length > 0 && shown.length === 0 && (
        <div className="empty-package">
          <SearchIcon />
          <h2>Nenhuma tarefa encontrada</h2>
          <p>Tente buscar por outro nome.</p>
        </div>
      )}
      {shown.length > 0 && (
        <div
          className={columns === 1 ? 'task-grid task-grid--list' : 'task-grid'}
          data-cols={columns}
          style={{ '--cols': columns } as CSSProperties}
        >
          {shown.map((task, index) => {
            const stats = task.stats
            return (
              <Link
                className="task-card"
                key={task.name}
                style={{ animationDelay: `${Math.min(index, 10) * 45}ms` }}
                to={`/tarefas/${encodeURIComponent(task.name)}`}
              >
                <div className="task-card__head">
                  <h2>{task.name}</h2>
                  <p className="task-card__description">{task.description || 'Sem descrição'}</p>
                </div>
                <div className="task-card__labels">
                  {task.labels.slice(0, 8).map((label) => (
                    <span className="label-pill" key={label}>
                      <i style={{ background: colorOf(label, task.name) }} />{label}
                    </span>
                  ))}
                  {task.labels.length > 8 && <span className="label-pill label-pill--more">+{task.labels.length - 8}</span>}
                  {task.labels.length === 0 && <span className="tasks-hint">Sem labels</span>}
                </div>
                <dl className="task-card__stats">
                  <div><dt>Imagens</dt><dd>{fmt(stats?.images ?? 0)}</dd></div>
                  <div><dt>Verificadas</dt><dd>{fmt(stats?.verified ?? 0)}</dd></div>
                  <div><dt>Caixas</dt><dd>{fmt(stats?.boxes ?? 0)}</dd></div>
                </dl>
              </Link>
            )
          })}
        </div>
      )}
      {creating && <NewTaskModal onClose={() => setCreating(false)} />}
    </section>
  )
}

// ── Pop-ups ─────────────────────────────────────────────────────────────

// A moldura dos pop-ups da tela (no body: um ancestral animado prenderia o
// position: fixed). Esc e o fundo fecham, menos no meio de um pedido.
function Modal({ title, busy = false, onClose, children, wide = false }: {
  title: string
  busy?: boolean
  onClose: () => void
  children: ReactNode
  wide?: boolean
}) {
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])
  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={() => { if (!busy) onClose() }} role="dialog">
      <div className={wide ? 'modal-dialog import-modal tasks-modal tasks-modal--wide' : 'modal-dialog import-modal tasks-modal'} onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>{title}</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy} onClick={onClose} type="button">✕</button>
        </div>
        {children}
      </div>
    </div>,
    document.body,
  )
}

// Digitar labels: Enter, vírgula ou colar uma lista viram etiquetas.
function LabelInput({ labels, onChange }: { labels: string[]; onChange: (labels: string[]) => void }) {
  const [text, setText] = useState('')

  function commit(raw: string) {
    const added = splitLabels(raw).filter((label) => !labels.includes(label))
    if (added.length > 0) onChange([...labels, ...added])
    setText('')
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter' || event.key === ',') {
      event.preventDefault()
      commit(text)
    } else if (event.key === 'Backspace' && text === '' && labels.length > 0) {
      onChange(labels.slice(0, -1))
    }
  }

  return (
    <div className="label-input">
      {labels.map((label) => (
        <span className="label-pill" key={label}>
          <i style={{ background: autoColor(label) }} />{label}
          <button aria-label={`Tirar ${label}`} onClick={() => onChange(labels.filter((item) => item !== label))} type="button">✕</button>
        </span>
      ))}
      <input
        onBlur={() => commit(text)}
        onChange={(event) => {
          const value = event.target.value
          // colou uma lista: vira etiquetas na hora
          if (/[\n,;]/.test(value)) commit(value)
          else setText(value)
        }}
        onKeyDown={onKeyDown}
        placeholder={labels.length === 0 ? 'ex.: com_capacete, sem_capacete' : 'mais uma...'}
        value={text}
      />
    </div>
  )
}

function NewTaskModal({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [labels, setLabels] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const nameOk = NAME_RE.test(name.trim())

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const created = await createTask({ name: name.trim(), description: description.trim(), labels })
      navigate(`/tarefas/${encodeURIComponent(created.name)}`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível criar a tarefa.')
      setBusy(false)
    }
  }

  return (
    <Modal busy={busy} onClose={onClose} title="Nova tarefa">
      <form onSubmit={(event) => void submit(event)}>
        <div className="modal-body">
          <label className="tasks-field">
            <span>Nome</span>
            <input autoFocus onChange={(event) => setName(event.target.value)} placeholder="ex.: capacete" value={name} />
            {name.trim() !== '' && !nameOk && <small className="tasks-field__bad">{NAME_HINT}</small>}
          </label>
          <label className="tasks-field">
            <span>Descrição (opcional)</span>
            <textarea onChange={(event) => setDescription(event.target.value)} placeholder="O que esta tarefa rotula" rows={2} value={description} />
          </label>
          <div className="tasks-field">
            <span>Labels iniciais</span>
            <LabelInput labels={labels} onChange={setLabels} />
            <small>Enter ou vírgula separa. Labels novas que vierem no import entram sozinhas.</small>
          </div>
          {error && <p className="form-error">{error}</p>}
        </div>
        <div className="modal-actions">
          <button className="button button--secondary" disabled={busy} onClick={onClose} type="button">Cancelar</button>
          <button className="button button--primary" disabled={busy || !nameOk} type="submit">{busy ? 'Criando...' : 'Criar tarefa'}</button>
        </div>
      </form>
    </Modal>
  )
}

// Baixar as imagens para rotular: escolhendo quais pacotes entram.
function DownloadModal({ task, onClose }: { task: AnnotationTask; onClose: () => void }) {
  const [packages, setPackages] = useState<PackageSummary[] | null>(null)
  const [chosen, setChosen] = useState<string[]>([])
  const [error, setError] = useState('')
  const inTask = useMemo(() => new Map((task.stats?.packages ?? []).map((item) => [item.name, item.images])), [task])

  useEffect(() => {
    let active = true
    listPackageSummaries()
      .then((list) => { if (active) setPackages(list.filter((item) => item.total > 0)) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os pacotes.')
      })
    return () => { active = false }
  }, [])

  const toggle = (name: string) => setChosen((list) => (list.includes(name) ? list.filter((item) => item !== name) : [...list, name]))
  const all = packages !== null && packages.length > 0 && chosen.length === packages.length
  const images = (packages ?? []).filter((item) => chosen.includes(item.name)).reduce((sum, item) => sum + item.total, 0)

  return (
    <Modal onClose={onClose} title="Baixar imagens para rotular" wide>
      <div className="modal-body">
        <p className="import-modal__intro">
          Um zip com as imagens dos pacotes escolhidos. Os nomes dos arquivos começam pelo sha1: é assim que o import das caixas encontra cada imagem depois.
        </p>
        {error && <p className="form-error">{error}</p>}
        {packages === null && !error && <PageLoader inline label="Carregando os pacotes" />}
        {packages && packages.length === 0 && <p className="tasks-hint">Nenhum pacote com imagens.</p>}
        {packages && packages.length > 0 && (
          <>
            <label className="package-pick package-pick--all">
              <input checked={all} onChange={() => setChosen(all ? [] : packages.map((item) => item.name))} type="checkbox" />
              <strong>Todos os pacotes</strong>
            </label>
            <ul className="package-picks">
              {packages.map((item) => (
                <li key={item.name}>
                  <label className="package-pick">
                    <input checked={chosen.includes(item.name)} onChange={() => toggle(item.name)} type="checkbox" />
                    <span className="package-pick__name">{item.name}</span>
                    <small>
                      {fmt(item.total)} imagem(ns)
                      {inTask.has(item.name) && ` · ${fmt(inTask.get(item.name) ?? 0)} já com caixas nesta tarefa`}
                    </small>
                  </label>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
      <div className="modal-actions">
        <span className="tasks-modal__summary">{chosen.length > 0 ? `${fmt(chosen.length)} pacote(s) · ${fmt(images)} imagem(ns)` : 'Escolha os pacotes'}</span>
        <button className="button button--secondary" onClick={onClose} type="button">Fechar</button>
        {chosen.length > 0 ? (
          <a className="button button--primary" download href={imagesZipUrl(chosen)}>Baixar zip</a>
        ) : (
          <button className="button button--primary" disabled type="button">Baixar zip</button>
        )}
      </div>
    </Modal>
  )
}

function DeleteModal({ task, onClose }: { task: AnnotationTask; onClose: () => void }) {
  const navigate = useNavigate()
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function remove(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      await deleteTask(task.name)
      navigate('/tarefas')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar a tarefa.')
      setBusy(false)
    }
  }

  return (
    <Modal busy={busy} onClose={onClose} title="Apagar tarefa">
      <form onSubmit={(event) => void remove(event)}>
        <div className="modal-body">
          <p className="modal-warning">
            Apaga a tarefa e as caixas de <strong>{fmt(task.stats?.images ?? 0)} imagem(ns)</strong>. As imagens continuam; só as caixas desta tarefa somem. Não dá para desfazer.
          </p>
          <label className="tasks-field">
            <span>Digite <code>{task.name}</code> para confirmar</span>
            <input autoFocus onChange={(event) => setTyped(event.target.value)} placeholder={task.name} value={typed} />
          </label>
          {error && <p className="form-error">{error}</p>}
        </div>
        <div className="modal-actions">
          <button className="button button--secondary" disabled={busy} onClick={onClose} type="button">Cancelar</button>
          <button className="button button--danger" disabled={busy || typed.trim() !== task.name} type="submit">{busy ? 'Apagando...' : 'Apagar de vez'}</button>
        </div>
      </form>
    </Modal>
  )
}

// ── Uma tarefa ──────────────────────────────────────────────────────────
type Popup = '' | 'import' | 'download' | 'delete'

function TaskDetail({ name }: { name: string }) {
  const navigate = useNavigate()
  const admin = useIsAdmin() // apagar é só de admin
  const [task, setTask] = useState<AnnotationTask | null>(null)
  const [error, setError] = useState('')
  const [popup, setPopup] = useState<Popup>('')
  const [message, setMessage] = useState('')

  const reload = useCallback(async () => {
    const fresh = await getTask(name)
    setTask(fresh)
    setTaskColors(fresh.name, fresh.colors)
  }, [name])

  useEffect(() => {
    let active = true
    getTask(name)
      .then((result) => { if (active) setTask(result) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível abrir a tarefa.')
      })
    return () => { active = false }
  }, [name])

  // Grava a tarefa inteira (o PUT troca tudo). Renomear muda a rota.
  async function save(changes: Partial<Pick<AnnotationTask, 'name' | 'description' | 'labels' | 'colors'>>) {
    if (!task) return
    const body = {
      name: changes.name ?? task.name,
      description: changes.description ?? task.description,
      labels: changes.labels ?? task.labels,
      colors: changes.colors ?? task.colors ?? {},
    }
    const saved = await updateTask(task.name, body)
    setTaskColors(saved.name, saved.colors)
    if (saved.name !== task.name) {
      navigate(`/tarefas/${encodeURIComponent(saved.name)}`, { replace: true })
      return
    }
    await reload()
  }

  if (error) {
    return (
      <section className="tasks-page">
        <button className="back-button" onClick={() => navigate('/tarefas')} type="button">← Tarefas</button>
        <p className="form-error">{error}</p>
      </section>
    )
  }
  if (!task) return <section className="tasks-page"><PageLoader label="Carregando a tarefa" /></section>

  const stats = task.stats
  return (
    <section className="tasks-page">
      <button className="back-button" onClick={() => navigate('/tarefas')} type="button">← Tarefas</button>
      <div className="page-heading task-heading">
        <TaskTitle onSave={save} task={task} />
        <div className="task-heading__actions">
          <button className="button button--secondary" onClick={() => setPopup('download')} type="button">Baixar imagens para rotular</button>
          <button className="button button--primary" onClick={() => { setMessage(''); setPopup('import') }} type="button">Importar caixas</button>
        </div>
      </div>
      {message && <p className="side-section__message">{message}</p>}

      <div className="detail-stats task-stats">
        <div><strong>{fmt(stats?.images ?? 0)}</strong><span>Imagens rotuladas</span></div>
        <div><strong>{fmt(stats?.verified ?? 0)}</strong><span>Verificadas (entram no export)</span></div>
        <div><strong>{fmt(stats?.negatives ?? 0)}</strong><span>Sem objetos</span></div>
        <div><strong>{fmt(stats?.boxes ?? 0)}</strong><span>Caixas</span></div>
      </div>

      <div className="task-layout">
        <LabelsCard key={`${task.labels.join('|')}#${JSON.stringify(task.colors ?? {})}`} onSave={save} task={task} />
        <section className="side-section task-packages">
          <div className="side-section__head">
            <h2>Pacotes</h2>
            <span>{fmt(stats?.packages.length ?? 0)} com caixas</span>
          </div>
          {(stats?.packages.length ?? 0) === 0 ? (
            <p className="side-section__hint">Nenhuma imagem rotulada nesta tarefa ainda. Baixe as imagens, rotule e importe as caixas.</p>
          ) : (
            <table className="task-table">
              <thead><tr><th>Pacote</th><th>Rotuladas</th><th>Verificadas</th></tr></thead>
              <tbody>
                {stats?.packages.map((row) => (
                  <tr key={row.name}>
                    <td><Link to={`/pacotes/${encodeURIComponent(row.name)}`}>{row.name}</Link></td>
                    <td>{fmt(row.images)}</td>
                    <td>{fmt(row.verified)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p className="side-section__hint task-packages__note">Só as imagens verificadas entram no export.</p>
          {admin && <button className="button button--danger button--sm task-delete" onClick={() => setPopup('delete')} type="button">Apagar tarefa</button>}
        </section>
      </div>

      {popup === 'import' && (
        <BoxImportModal
          defaultTask={task.name}
          fixedTask
          onApplied={(text) => { setPopup(''); setMessage(text); void reload() }}
          onClose={() => setPopup('')}
        />
      )}
      {popup === 'download' && <DownloadModal onClose={() => setPopup('')} task={task} />}
      {popup === 'delete' && <DeleteModal onClose={() => setPopup('')} task={task} />}
    </section>
  )
}

// O nome e a descrição, com o lápis para editar no lugar.
function TaskTitle({ task, onSave }: { task: AnnotationTask; onSave: (changes: { name: string; description: string }) => Promise<void> }) {
  const admin = useIsAdmin() // nome e descrição: só admin
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(task.name)
  const [description, setDescription] = useState(task.description)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const nameOk = NAME_RE.test(name.trim())

  function start() {
    setName(task.name)
    setDescription(task.description)
    setError('')
    setEditing(true)
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await onSave({ name: name.trim(), description: description.trim() })
      setEditing(false)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar.')
    } finally {
      setBusy(false)
    }
  }

  if (!editing) {
    return (
      <div className="task-title">
        <span className="eyebrow">Tarefa</span>
        <div className="task-title__row">
          <h1>{task.name}</h1>
          {admin && (
            <button aria-label="Editar o nome e a descrição" className="task-title__edit" onClick={start} title="Editar o nome e a descrição" type="button">
              <svg aria-hidden="true" height="16" viewBox="0 0 24 24" width="16"><path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16v4Z" fill="none" stroke="currentColor" strokeLinejoin="round" strokeWidth="1.8" /></svg>
            </button>
          )}
        </div>
        <p>{task.description || <span className="tasks-hint">Sem descrição</span>}</p>
      </div>
    )
  }

  return (
    <form className="task-title task-title--editing" onSubmit={(event) => void submit(event)}>
      <span className="eyebrow">Tarefa</span>
      <input aria-label="Nome" autoFocus className="task-title__name" onChange={(event) => setName(event.target.value)} value={name} />
      {name.trim() !== '' && !nameOk && <small className="tasks-field__bad">{NAME_HINT}</small>}
      {nameOk && name.trim() !== task.name && <small className="tasks-hint">Renomear leva junto as caixas e os perfis de export que usam esta tarefa.</small>}
      <textarea aria-label="Descrição" onChange={(event) => setDescription(event.target.value)} placeholder="Descrição (opcional)" rows={2} value={description} />
      {error && <p className="form-error">{error}</p>}
      <div className="task-title__actions">
        <button className="button button--primary button--sm" disabled={busy || !nameOk} type="submit">{busy ? 'Salvando...' : 'Salvar'}</button>
        <button className="button button--secondary button--sm" disabled={busy} onClick={() => setEditing(false)} type="button">Cancelar</button>
      </div>
    </form>
  )
}

// As labels: a cor de cada uma (clicar na bolinha), quantas caixas tem,
// tirar e acrescentar. Mudanças ficam num rascunho até salvar.
function LabelsCard({ task, onSave }: { task: AnnotationTask; onSave: (changes: { labels: string[]; colors: Record<string, string> }) => Promise<void> }) {
  const admin = useIsAdmin() // mudar labels e cores: só admin (o usuário vê)
  const [labels, setLabels] = useState(task.labels)
  const [colors, setColors] = useState<Record<string, string>>(task.colors ?? {})
  const [adding, setAdding] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const counts = task.stats?.labels ?? {}
  const totalBoxes = task.stats?.boxes ?? 0
  const dirty = labels.join('\n') !== task.labels.join('\n') || JSON.stringify(sorted(colors)) !== JSON.stringify(sorted(task.colors ?? {}))
  // têm caixas mas saíram da lista (as caixas continuam)
  const orphans = Object.keys(counts).filter((label) => !labels.includes(label))

  function add(event: FormEvent) {
    event.preventDefault()
    const added = splitLabels(adding).filter((label) => !labels.includes(label))
    setLabels([...labels, ...added])
    setAdding('')
  }

  function remove(label: string) {
    setLabels(labels.filter((item) => item !== label))
    const next = { ...colors }
    delete next[label]
    setColors(next)
  }

  async function submit() {
    setBusy(true)
    setError('')
    try {
      await onSave({ labels, colors })
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar as labels.')
      setBusy(false)
    }
  }

  return (
    <section className="side-section task-labels">
      <div className="side-section__head">
        <h2>Labels</h2>
        <span>{fmt(labels.length)}{admin ? ' · clique na cor para trocar' : ''}</span>
      </div>
      {labels.length === 0 && <p className="side-section__hint">Nenhuma label. Acrescente abaixo, ou elas entram sozinhas no import.</p>}
      <ul className="label-rows">
        {labels.map((label) => {
          const color = colors[label] ?? colorOf(label, task.name)
          const custom = Boolean(colors[label])
          const boxes = counts[label] ?? 0
          return (
            <li className="label-row" key={label}>
              {admin ? (
                <label className="label-swatch" style={{ background: color }} title="Trocar a cor">
                  <input onChange={(event) => setColors({ ...colors, [label]: event.target.value })} type="color" value={toHex(color)} />
                </label>
              ) : (
                <span className="label-swatch label-swatch--static" style={{ background: color }} />
              )}
              <span className="label-row__name">{label}</span>
              <span className="label-row__bar"><span style={{ width: pct(boxes, totalBoxes), background: color }} /></span>
              <span className="label-row__count">{fmt(boxes)} <small>{pct(boxes, totalBoxes)}</small></span>
              {admin && <span className="label-row__tools">
                {custom && (
                  <button
                    onClick={() => { const next = { ...colors }; delete next[label]; setColors(next) }}
                    title={`Voltar à cor automática (${autoColor(label)})`}
                    type="button"
                  >
                    ↺
                  </button>
                )}
                <button
                  aria-label={`Tirar ${label}`}
                  onClick={() => remove(label)}
                  title={boxes > 0 ? 'Tirar da lista (as caixas continuam; no export, escolha se entram)' : 'Tirar da lista'}
                  type="button"
                >
                  ✕
                </button>
              </span>}
            </li>
          )
        })}
      </ul>
      {orphans.length > 0 && (
        <p className="import-report__detail import-report__detail--warning">
          Têm caixas mas não estão na lista: {orphans.join(', ')}. As caixas continuam; no export, escolha se entram.
        </p>
      )}
      {admin && <form className="label-add" onSubmit={add}>
        <input onChange={(event) => setAdding(event.target.value)} placeholder="Nova label (ou várias, separadas por vírgula)" value={adding} />
        <button className="button button--secondary button--sm" disabled={!adding.trim()} type="submit">Acrescentar</button>
      </form>}
      {error && <p className="form-error">{error}</p>}
      {admin && dirty && (
        <div className="label-save">
          <span>Alterações não salvas</span>
          <button className="button button--secondary button--sm" disabled={busy} onClick={() => { setLabels(task.labels); setColors(task.colors ?? {}) }} type="button">Desfazer</button>
          <button className="button button--primary button--sm" disabled={busy} onClick={() => void submit()} type="button">{busy ? 'Salvando...' : 'Salvar labels'}</button>
        </div>
      )}
    </section>
  )
}

function sorted(map: Record<string, string>) {
  return Object.keys(map).sort().map((key) => [key, map[key]])
}

// o <input type="color"> só aceita #rrggbb
function toHex(color: string): string {
  return /^#[0-9a-f]{6}$/i.test(color) ? color.toLowerCase() : '#888888'
}
