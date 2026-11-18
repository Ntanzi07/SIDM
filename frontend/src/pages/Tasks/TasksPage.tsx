import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  createTask,
  cvatZipUrl,
  deleteTask,
  getTask,
  importTaskBoxes,
  listPackageNames,
  listTasks,
  updateTask,
  type AnnotationTask,
  type BoxImportReport,
} from '../../services/api'
import '../Export/Export.css'
import './Tasks.css'

const fmt = (n: number) => n.toLocaleString('pt-BR')
const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/

// "a, b\nc" -> ["a", "b", "c"] (sem vazias nem repetidas, na ordem).
function parseLabels(text: string): string[] {
  const out: string[] = []
  for (const part of text.split(/[\n,]/)) {
    const label = part.trim()
    if (label && !out.includes(label)) out.push(label)
  }
  return out
}

function labelList(labels: Record<string, number>) {
  return Object.entries(labels)
    .sort(([, a], [, b]) => b - a)
    .map(([label, n]) => `${label} ${fmt(n)}`)
    .join(' · ')
}

const LETTERS_AND_DIGITS = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ', ...'0123456789']

type Draft = { name: string; description: string; labels: string }

const emptyDraft: Draft = { name: '', description: '', labels: '' }

// Tarefas de anotação de caixas: cada uma tem as suas labels e as suas
// caixas (a mesma imagem pode estar em várias). Os pacotes de uma tarefa são
// os das imagens anotadas nela.
export function TasksPage() {
  const { name: selected } = useParams()
  const [tasks, setTasks] = useState<AnnotationTask[]>([])
  const [listError, setListError] = useState('')
  const [message, setMessage] = useState('')

  const reloadList = useCallback(async () => {
    try {
      setTasks(await listTasks())
      setListError('')
    } catch (requestError: unknown) {
      setListError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as tarefas.')
    }
  }, [])

  useEffect(() => {
    let active = true
    listTasks()
      .then((list) => { if (active) setTasks(list) })
      .catch((requestError: unknown) => {
        if (active) setListError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as tarefas.')
      })
    return () => { active = false }
  }, [])

  return (
    <section className="export-page tasks-page">
      <div className="page-heading">
        <div>
          <h1>Tarefas</h1>
          <p>Cada tarefa tem as suas labels e as suas caixas do CVAT (ex.: capacete, letras da placa). A mesma imagem pode estar em várias.</p>
        </div>
      </div>
      {listError && <p className="form-error">{listError}</p>}
      {message && <p className="settings-message">{message}</p>}

      <div className="export-layout">
        <aside className="export-profiles">
          <span className="export-profiles__title">Tarefas</span>
          {tasks.map((item) => (
            <Link
              className={item.name === selected ? 'export-profile export-profile--active' : 'export-profile'}
              key={item.name}
              onClick={() => setMessage('')}
              to={`/tarefas/${encodeURIComponent(item.name)}`}
            >
              <strong>{item.name}</strong>
              <span>{fmt(item.stats?.images ?? 0)} imagem(ns) · {fmt(item.labels.length)} label(s)</span>
            </Link>
          ))}
          {tasks.length === 0 && <span className="profile-name-hint">Nenhuma tarefa ainda.</span>}
          <Link className="button button--secondary export-profiles__new" onClick={() => setMessage('')} to="/tarefas">+ Nova tarefa</Link>
        </aside>

        <div className="export-editor">
          <TaskEditor key={selected ?? ''} name={selected} onChanged={reloadList} onMessage={setMessage} />
        </div>
      </div>
    </section>
  )
}

function draftOf(task: AnnotationTask): Draft {
  return { name: task.name, description: task.description, labels: task.labels.join('\n') }
}

// O formulário de uma tarefa (ou de uma nova, sem name). Remontado a cada
// tarefa aberta (key), então começa sempre do zero.
function TaskEditor({ name, onChanged, onMessage }: {
  name: string | undefined
  onChanged: () => Promise<void>
  onMessage: (message: string) => void
}) {
  const navigate = useNavigate()
  const [task, setTask] = useState<AnnotationTask | null>(null)
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [busy, setBusy] = useState<'' | 'save' | 'delete'>('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!name) return
    let active = true
    getTask(name)
      .then((result) => {
        if (!active) return
        setTask(result)
        setDraft(draftOf(result))
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível abrir a tarefa.')
      })
    return () => { active = false }
  }, [name])

  async function refreshTask(taskName: string) {
    const [fresh] = await Promise.all([getTask(taskName), onChanged()])
    setTask(fresh)
    setDraft(draftOf(fresh))
  }

  async function save(event: FormEvent) {
    event.preventDefault()
    setBusy('save')
    setError('')
    onMessage('')
    const body = { name: draft.name.trim(), description: draft.description.trim(), labels: parseLabels(draft.labels) }
    try {
      if (task) {
        const saved = await updateTask(task.name, body)
        if (saved.name !== task.name) {
          await onChanged()
          onMessage(`Tarefa renomeada para "${saved.name}".`)
          navigate(`/tarefas/${encodeURIComponent(saved.name)}`, { replace: true })
        } else {
          await refreshTask(saved.name)
          onMessage('Tarefa salva.')
        }
      } else {
        const created = await createTask(body)
        await onChanged()
        onMessage(`Tarefa "${created.name}" criada. Agora importe as caixas do CVAT.`)
        navigate(`/tarefas/${encodeURIComponent(created.name)}`)
      }
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar a tarefa.')
    } finally {
      setBusy('')
    }
  }

  async function remove() {
    if (!task) return
    const images = task.stats?.images ?? 0
    const typed = window.prompt(
      `Apagar a tarefa "${task.name}" e as caixas de ${fmt(images)} imagem(ns)? As imagens continuam; só as caixas desta tarefa somem.\n\nDigite o nome da tarefa para confirmar:`,
    )
    if (typed === null) return
    if (typed.trim() !== task.name) {
      setError('O nome digitado não confere: nada foi apagado.')
      return
    }
    setBusy('delete')
    setError('')
    try {
      await deleteTask(task.name)
      await onChanged()
      onMessage(`Tarefa "${task.name}" apagada.`)
      navigate('/tarefas')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar a tarefa.')
      setBusy('')
    }
  }

  if (name && !task) {
    return error ? <p className="form-error">{error}</p> : <p className="profile-name-hint">Carregando…</p>
  }

  const nameOk = NAME_RE.test(draft.name.trim())
  const labels = parseLabels(draft.labels)
  const dirty = task
    ? draft.name.trim() !== task.name || draft.description.trim() !== task.description || labels.join('\n') !== task.labels.join('\n')
    : true
  const stats = task?.stats
  // Labels com caixas que não estão na lista da tarefa (tiradas da lista depois do import).
  const orphanLabels = stats ? Object.keys(stats.labels).filter((label) => !task?.labels.includes(label)) : []

  return (
    <>
      <form className="create-card settings-card" onSubmit={(event) => void save(event)}>
        <h2>{task ? task.name : 'Nova tarefa'}</h2>
        {error && <p className="form-error">{error}</p>}
        {stats && (
          <div className="import-report__stats">
            <span><strong>{fmt(stats.images)}</strong> imagens anotadas</span>
            <span><strong>{fmt(stats.verified)}</strong> verificadas (entram no export)</span>
            <span><strong>{fmt(stats.negatives)}</strong> sem objetos</span>
            <span><strong>{fmt(stats.boxes)}</strong> caixas</span>
          </div>
        )}
        <div className="profile-form-grid">
          <label className="form-field">
            <span>Nome</span>
            <input onChange={(event) => setDraft({ ...draft, name: event.target.value })} placeholder="Ex.: placa-letras" value={draft.name} />
            {draft.name.trim() !== '' && !nameOk && (
              <small className="profile-name-hint tasks-bad">Só letras minúsculas, números, - e _ (sem espaço).</small>
            )}
            {task && draft.name.trim() !== task.name && nameOk && (
              <small className="profile-name-hint">Renomear leva junto as caixas e os perfis de export que usam esta tarefa.</small>
            )}
          </label>
          <label className="form-field">
            <span>Descrição (opcional)</span>
            <input onChange={(event) => setDraft({ ...draft, description: event.target.value })} value={draft.description} />
          </label>
        </div>
        <label className="form-field tasks-labels">
          <span>Labels ({labels.length}) — uma por linha, como estão no CVAT</span>
          <textarea
            onChange={(event) => setDraft({ ...draft, labels: event.target.value })}
            placeholder={'com_capacete\nsem_capacete'}
            rows={Math.min(12, Math.max(4, labels.length + 1))}
            value={draft.labels}
          />
        </label>
        <div className="tasks-label-tools">
          <button
            className="button button--secondary button--sm"
            onClick={() => setDraft({ ...draft, labels: [...labels, ...LETTERS_AND_DIGITS.filter((c) => !labels.includes(c))].join('\n') })}
            type="button"
          >
            + A–Z e 0–9
          </button>
          <span className="profile-name-hint">Labels novas que vierem do CVAT entram na lista sozinhas, no import.</span>
        </div>
        {orphanLabels.length > 0 && (
          <p className="import-report__detail import-report__detail--warning">
            Têm caixas mas não estão na lista: {orphanLabels.join(', ')}. As caixas continuam; no export, escolha se entram.
          </p>
        )}
        <div className="export-actions">
          <button className="button button--primary" disabled={busy !== '' || !nameOk || !dirty} type="submit">
            {busy === 'save' ? 'Salvando...' : task ? 'Salvar alterações' : 'Criar tarefa'}
          </button>
          <span className="export-actions__spacer" />
          {task && (
            <button className="button button--danger" disabled={busy !== ''} onClick={() => void remove()} type="button">
              {busy === 'delete' ? 'Apagando...' : 'Apagar tarefa'}
            </button>
          )}
        </div>
      </form>

      {task && stats && (
        <>
          <TaskPackages task={task} />
          {stats.boxes > 0 && (
            <div className="create-card settings-card">
              <h2>Caixas por label</h2>
              <p className="import-report__detail">{labelList(stats.labels)}</p>
            </div>
          )}
          <TaskImport onApplied={() => refreshTask(task.name)} task={task.name} />
        </>
      )}
    </>
  )
}

// Os pacotes que têm imagens na tarefa, e o zip de qualquer pacote para
// mandar ao CVAT.
function TaskPackages({ task }: { task: AnnotationTask }) {
  const [packages, setPackages] = useState<string[]>([])
  const [zipPackage, setZipPackage] = useState('')

  useEffect(() => {
    let active = true
    listPackageNames()
      .then((names) => { if (active) setPackages(names) })
      .catch(() => { /* só para o zip */ })
    return () => { active = false }
  }, [])

  const rows = task.stats?.packages ?? []
  return (
    <div className="create-card settings-card">
      <h2>Pacotes</h2>
      {rows.length === 0 ? (
        <p className="profile-name-hint">Nenhuma imagem anotada nesta tarefa ainda. Os pacotes aparecem aqui depois do import.</p>
      ) : (
        <table className="export-labels">
          <thead><tr><th>Pacote</th><th>Anotadas</th><th>Verificadas</th></tr></thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.name}>
                <td><Link to={`/pacotes/${encodeURIComponent(row.name)}`}>{row.name}</Link></td>
                <td>{fmt(row.images)}</td>
                <td>{fmt(row.verified)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="profile-name-hint">Só as verificadas entram no export.</p>
      <h3>Mandar imagens para o CVAT</h3>
      <div className="existing-import__controls">
        <select aria-label="Pacote" onChange={(event) => setZipPackage(event.target.value)} value={zipPackage}>
          <option value="">Escolha o pacote…</option>
          {packages.map((name) => <option key={name} value={name}>{name}</option>)}
        </select>
        {zipPackage ? (
          <a className="button button--secondary" download href={cvatZipUrl(zipPackage)}>Baixar imagens para o CVAT</a>
        ) : (
          <button className="button button--secondary" disabled type="button">Baixar imagens para o CVAT</button>
        )}
      </div>
      <p className="profile-name-hint">
        Os nomes dos arquivos começam pelo sha1: é assim que o import encontra cada imagem, em qualquer pacote.
      </p>
    </div>
  )
}

// Import do export COCO 1.0 do CVAT: ensaio primeiro, depois gravar.
function TaskImport({ task, onApplied }: { task: string; onApplied: () => Promise<void> }) {
  const [file, setFile] = useState<File | null>(null)
  const [negatives, setNegatives] = useState(false)
  const [report, setReport] = useState<BoxImportReport | null>(null)
  const [saving, setSaving] = useState<'' | 'testing' | 'applying'>('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  async function run(dryRun: boolean) {
    if (!file) return
    setSaving(dryRun ? 'testing' : 'applying')
    setError('')
    setMessage('')
    try {
      const result = await importTaskBoxes(task, file, dryRun, negatives)
      if (dryRun) {
        setReport(result)
      } else {
        setReport(null)
        setFile(null)
        setMessage(`${fmt(result.applied_boxes)} caixa(s) gravada(s) em ${fmt(result.applied)} imagem(ns).`)
        await onApplied()
      }
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível importar as caixas.')
    } finally {
      setSaving('')
    }
  }

  return (
    <form
      className="create-card settings-card"
      onSubmit={(event) => {
        event.preventDefault()
        void run(true)
      }}
    >
      <h2>Importar caixas do CVAT</h2>
      <p className="profile-name-hint">
        Exporte a task do CVAT em <strong>COCO 1.0</strong> e mande o zip (ou só o .json). As imagens são procuradas em todos os pacotes.
      </p>
      <div className="existing-import__controls">
        <label className="existing-import__file">
          <span>{file?.name ?? 'Selecione o export do CVAT (COCO 1.0: .zip ou .json)'}</span>
          <input
            accept=".zip,.json,application/zip,application/json"
            disabled={saving !== ''}
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null)
              setReport(null)
              setError('')
              setMessage('')
            }}
            type="file"
          />
        </label>
        <button className="button button--secondary" disabled={saving !== '' || !file} type="submit">
          {saving === 'testing' ? 'Ensaiando...' : 'Ensaiar import'}
        </button>
      </div>
      <label className="export-check tasks-negatives">
        <input
          checked={negatives}
          onChange={(event) => {
            setNegatives(event.target.checked)
            setReport(null)
          }}
          type="checkbox"
        />
        Imagens sem nenhuma caixa entram como "sem objetos"
      </label>
      <p className="profile-name-hint">
        Marque só se a task do CVAT estiver <strong>toda anotada</strong>: o CVAT exporta todas as imagens, inclusive as que ninguém olhou ainda.
        Desmarcado, essas imagens são ignoradas.
      </p>
      {error && <p className="form-error">{error}</p>}
      {message && <p className="form-hint">{message}</p>}
      {report && (
        <div className="import-report">
          <strong>Ensaio das caixas</strong>
          <div className="import-report__stats">
            <span><strong>{fmt(report.file_images)}</strong> imagens no arquivo</span>
            <span><strong>{fmt(report.matched)}</strong> encontradas no SIDM</span>
            <span><strong>{fmt(report.will_apply)}</strong> serão gravadas</span>
            <span><strong>{fmt(report.not_found_count)}</strong> não encontradas</span>
          </div>
          {Object.keys(report.packages).length > 0 && (
            <p className="import-report__detail"><strong>Por pacote:</strong> {labelList(report.packages)}</p>
          )}
          {Object.keys(report.labels).length > 0 && (
            <p className="import-report__detail"><strong>Caixas por label:</strong> {labelList(report.labels)}</p>
          )}
          {report.new_labels.length > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              Labels que a tarefa ainda não tem (serão acrescentadas): {report.new_labels.join(', ')}
            </p>
          )}
          {report.replaced > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              {fmt(report.replaced)} imagem(ns) já estão nesta tarefa: as caixas delas serão substituídas.
            </p>
          )}
          {report.edited_replaced > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              {fmt(report.edited_replaced)} imagem(ns) têm atributos editados no SIDM: o import do CVAT vai sobrescrever.
            </p>
          )}
          {report.attribute_problem_count > 0 && (
            <div className="import-report__detail import-report__detail--warning">
              <strong>{fmt(report.attribute_problem_count)} atributo(s) fora do perfil</strong> (ficam de fora):
              <ul>
                {report.attribute_problems.slice(0, 10).map((problem) => <li key={problem}>{problem}</li>)}
              </ul>
            </div>
          )}
          {report.missing_required > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              {fmt(report.missing_required)} caixa(s) sem atributo obrigatório: dá para completar na tela de anotação.
            </p>
          )}
          {report.untyped_labels.length > 0 && (
            <p className="import-report__detail">
              Labels sem tipo de objeto com caixa no perfil do pacote (atributos guardados como vieram): {report.untyped_labels.join(', ')}
            </p>
          )}
          {report.empty > 0 && (
            <p className="import-report__detail">
              {fmt(report.empty)} imagem(ns) sem nenhuma caixa no arquivo: {report.negatives ? 'entram como "sem objetos".' : 'ignoradas.'}
            </p>
          )}
          {report.not_found_count > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              <strong>Não encontradas no SIDM:</strong> {report.not_found.slice(0, 5).join(', ')}{report.not_found_count > 5 && ', …'}
              {' '}— crie a task do CVAT com o zip "Baixar imagens para o CVAT" para os nomes casarem.
            </p>
          )}
          {report.ambiguous.length > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              <strong>Nome repetido no SIDM (puladas):</strong> {report.ambiguous.slice(0, 5).join(', ')}
            </p>
          )}
          {report.size_mismatch.slice(0, 5).map((item) => (
            <p className="import-report__detail import-report__detail--warning" key={item.file_name}>
              {item.file_name}: tamanho no CVAT {item.cvat}, no SIDM {item.sidm} — pulada (as caixas ficariam fora do lugar).
            </p>
          ))}
          {report.warnings.slice(0, 5).map((warning) => (
            <p className="import-report__detail import-report__detail--warning" key={warning}>{warning}</p>
          ))}
          <button
            className="button button--primary"
            disabled={saving !== '' || report.will_apply === 0}
            onClick={() => void run(false)}
            type="button"
          >
            {saving === 'applying' ? 'Gravando...' : `Gravar caixas em ${fmt(report.will_apply)} imagem(ns)`}
          </button>
        </div>
      )}
    </form>
  )
}
