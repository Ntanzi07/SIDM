import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  cancelProcessingRun,
  listPackageSummaries,
  listProcessingModels,
  listProcessingRuns,
  listTasks,
  startProcessingRun,
  type PackageSummary,
} from '../../services/api'
import { confirmDialog } from '../../services/confirm'
import { notify } from '../../services/toast'
import { Skeleton } from '../../components/Skeleton/Skeleton'
import { SkFormCard, SkProfileList } from '../../components/Skeleton/Shapes'
import type { ProcessingModel, ProcessingRun, ProcessingScope, ProcessingStatus } from '../../types/processing'
import './Process.css'

// O layout segue o Exportar: à esquerda os modelos (e o histórico), à
// direita o cartão do que está escolhido.

const fmt = (n: number) => n.toLocaleString('pt-BR')

const SCOPES: { value: ProcessingScope; label: string; hint: string }[] = [
  { value: 'pending', label: 'Só pendentes', hint: 'as que ainda não foram conferidas' },
  { value: 'verified', label: 'Só verificadas', hint: 'as que já foram conferidas' },
  { value: 'all', label: 'Todas', hint: 'pendentes e verificadas' },
]

const SCOPE_TEXT: Record<ProcessingScope, string> = { pending: 'pendentes', verified: 'verificadas', all: 'todas' }
// ao lado de cada pacote: "(3 pendentes)", "(5 no total)"
const SCOPE_COUNT: Record<ProcessingScope, string> = { pending: 'pendentes', verified: 'verificadas', all: 'no total' }

const STATUS_TEXT: Record<ProcessingStatus, string> = {
  running: 'Rodando',
  done: 'Concluído',
  failed: 'Falhou',
  canceled: 'Cancelado',
  interrupted: 'Interrompido',
}

// quantas imagens de um pacote entram no escopo
function inScope(item: PackageSummary, scope: ProcessingScope): number {
  if (scope === 'pending') return item.pending
  if (scope === 'verified') return item.verified
  return item.total
}

function formatDate(value?: string): string {
  if (!value) return ''
  return new Date(value).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' })
}

// "2 min 05 s" entre o início e o fim (ou agora, se ainda roda)
function duration(run: ProcessingRun): string {
  const end = run.finished_at ? new Date(run.finished_at).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(run.started_at).getTime()) / 1000))
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return m > 0 ? `${m} min ${String(s).padStart(2, '0')} s` : `${s} s`
}

// o estado do modelo, embaixo do nome na lista da esquerda
function modelState(model: ProcessingModel): string {
  if (!model.available) return 'indisponível'
  if (model.busy) return 'rodando'
  return 'disponível'
}

const HISTORY = '__historico__'

const PROCESS_INTRO = 'Pré-anote as imagens com um modelo de IA: ele acha as caixas e grava numa tarefa, para alguém só corrigir.'

export function ProcessPage() {
  const [models, setModels] = useState<ProcessingModel[] | null>(null)
  const [packages, setPackages] = useState<PackageSummary[] | null>(null)
  const [tasks, setTasks] = useState<string[]>([])
  const [runs, setRuns] = useState<ProcessingRun[] | null>(null)
  const [error, setError] = useState('')

  // o que está aberto à direita: um modelo ou o histórico
  const [view, setView] = useState('')
  // o formulário
  const [chosen, setChosen] = useState<string[]>([])
  const [scope, setScope] = useState<ProcessingScope>('pending')
  const [task, setTask] = useState('')
  const [minScore, setMinScore] = useState('0.5')
  const [advanced, setAdvanced] = useState(false)
  const [starting, setStarting] = useState(false)

  const running = useRef<Set<string>>(new Set())

  // runs e modelos de novo; avisa quando um que estava rodando termina
  const refresh = useCallback(async () => {
    const [nextRuns, nextModels] = await Promise.all([listProcessingRuns(), listProcessingModels()])
    for (const run of nextRuns) {
      if (running.current.has(run.id) && run.status !== 'running') {
        if (run.status === 'done') notify(`Processamento concluído: ${fmt(run.boxes)} caixa(s) em ${fmt(run.written)} imagem(ns).`)
        else if (run.status === 'canceled') notify('Processamento cancelado.', 'info')
        else notify(`O processamento parou: ${run.message ?? STATUS_TEXT[run.status]}.`, 'error')
      }
    }
    running.current = new Set(nextRuns.filter((run) => run.status === 'running').map((run) => run.id))
    setRuns(nextRuns)
    setModels(nextModels)
  }, [])

  // abre um modelo com os valores padrão dele
  const openModel = useCallback((model: ProcessingModel) => {
    setView(model.key)
    setTask(model.default_task)
    setMinScore(String(model.default_min_score))
  }, [])

  useEffect(() => {
    let active = true
    Promise.all([listProcessingModels(), listPackageSummaries(), listTasks(), listProcessingRuns()])
      .then(([nextModels, nextPackages, nextTasks, nextRuns]) => {
        if (!active) return
        setModels(nextModels)
        setPackages(nextPackages)
        setTasks(nextTasks.map((item) => item.name))
        setRuns(nextRuns)
        running.current = new Set(nextRuns.filter((run) => run.status === 'running').map((run) => run.id))
        const first = nextModels.find((model) => model.available) ?? nextModels[0]
        if (first) openModel(first)
        else setView(HISTORY)
      })
      .catch((err: Error) => { if (active) setError(err.message) })
    return () => { active = false }
  }, [openModel])

  const current = runs?.find((run) => run.status === 'running')

  // enquanto algo roda, pergunta a cada 2 s
  useEffect(() => {
    if (!current) return
    const timer = window.setInterval(() => { refresh().catch(() => undefined) }, 2000)
    return () => window.clearInterval(timer)
  }, [current, refresh])

  if (error) {
    return (
      <section className="processing-page">
        <div className="page-heading"><div><h1>Processar</h1></div></div>
        <p className="form-error">{error}</p>
      </section>
    )
  }
  if (models === null || packages === null || runs === null) {
    // a silhueta: os modelos à esquerda e o formulário (o título é o de verdade)
    return (
      <section className="processing-page">
        <div className="page-heading"><div><h1>Processar</h1><p>{PROCESS_INTRO}</p></div></div>
        <Skeleton label="Carregando os modelos">
          <div className="export-layout">
            <SkProfileList count={2} />
            <div className="export-editor"><SkFormCard fields={4} /></div>
          </div>
        </Skeleton>
      </section>
    )
  }

  const model = models.find((item) => item.key === view)
  const modelLabel = (key: string) => models.find((item) => item.key === key)?.label ?? key
  const allChosen = packages.length > 0 && chosen.length === packages.length
  const imageCount = packages.filter((item) => chosen.includes(item.name)).reduce((sum, item) => sum + inScope(item, scope), 0)
  const score = Number(minScore.replace(',', '.'))
  const scoreOk = minScore.trim() !== '' && !Number.isNaN(score) && score >= 0 && score <= 1

  const toggle = (name: string) => setChosen((list) => (list.includes(name) ? list.filter((item) => item !== name) : [...list, name]))

  let blocked = ''
  if (model && !model.available) blocked = 'O modelo não está respondendo'
  else if (model?.busy) blocked = 'Este modelo já está processando'
  else if (chosen.length === 0) blocked = 'Escolha pelo menos um pacote'
  else if (imageCount === 0) blocked = `Nenhuma imagem ${SCOPE_TEXT[scope]} nos pacotes escolhidos`
  else if (!scoreOk) blocked = 'O score mínimo vai de 0 a 1'

  const start = async () => {
    if (!model || blocked) return
    setStarting(true)
    try {
      const run = await startProcessingRun({ model: model.key, packages: chosen, scope, task: task.trim() || undefined, min_score: score })
      notify(`Processamento iniciado: ${fmt(imageCount)} imagem(ns).`, 'info')
      running.current.add(run.id)
      await refresh()
      if (!tasks.includes(run.task)) setTasks((list) => [...list, run.task])
    } catch (err) {
      notify((err as Error).message, 'error')
      refresh().catch(() => undefined)
    } finally {
      setStarting(false)
    }
  }

  const cancel = async (run: ProcessingRun) => {
    const ok = await confirmDialog({
      title: 'Cancelar o processamento?',
      message: 'Ele para depois da imagem que está no modelo. As caixas já gravadas ficam na tarefa.',
      confirmLabel: 'Cancelar processamento',
      cancelLabel: 'Continuar rodando',
      tone: 'danger',
    })
    if (!ok) return
    try {
      await cancelProcessingRun(run.id)
      await refresh()
    } catch (err) {
      notify((err as Error).message, 'error')
    }
  }

  return (
    <section className="processing-page">
      <div className="page-heading">
        <div>
          <h1>Processar</h1>
          <p>{PROCESS_INTRO}</p>
        </div>
      </div>

      <div className="export-layout">
        <aside className="export-profiles">
          <span className="export-profiles__title">Modelos</span>
          {models.length === 0 && <p className="profile-name-hint">Nenhum modelo ligado.</p>}
          {models.map((item) => (
            <button
              className={item.key === view ? 'export-profile export-profile--active' : 'export-profile'}
              key={item.key}
              onClick={() => openModel(item)}
              type="button"
            >
              <strong>{item.label}</strong>
              <span className={`process-state process-state--${item.available ? (item.busy ? 'busy' : 'on') : 'off'}`}>{modelState(item)}</span>
            </button>
          ))}

          <span className="export-profiles__title process-aside-gap">Acompanhar</span>
          <button className={view === HISTORY ? 'export-profile export-profile--active' : 'export-profile'} onClick={() => setView(HISTORY)} type="button">
            <strong>Histórico</strong>
            <span>{runs.length === 0 ? 'nenhum ainda' : `${fmt(runs.length)} processamento(s)`}</span>
          </button>
        </aside>

        <div className="export-editor">
          {current && <CurrentRun label={modelLabel(current.model)} onCancel={() => { cancel(current) }} run={current} />}

          {view === HISTORY && <History modelLabel={modelLabel} runs={runs} />}

          {view !== HISTORY && !model && (
            <div className="create-card settings-card">
              <h2>Nenhum modelo ligado</h2>
              <p className="profile-name-hint">
                Defina no servidor o endereço do modelo (<code>LPR_ADDR</code> para o detector de placas, <code>AXLE_ADDR</code> para o contador de eixos) e reinicie o backend.
              </p>
            </div>
          )}

          {model && (
            <div className="create-card settings-card">
              <h2>{model.label}</h2>
              <p className="profile-name-hint">{model.description}</p>
              {!model.available && (
                <p className="form-error">O modelo não está respondendo. Confira se o container dele está de pé; o detalhe está no log do backend.</p>
              )}

              <h3>Pacotes</h3>
              {packages.length === 0 ? (
                <p className="profile-name-hint">Nenhum pacote ainda.</p>
              ) : (
                <div className="export-packages">
                  <label className="export-check">
                    <input checked={allChosen} onChange={() => setChosen(allChosen ? [] : packages.map((item) => item.name))} type="checkbox" />
                    Todos
                  </label>
                  {packages.map((item) => (
                    <label className="export-check" key={item.name}>
                      <input checked={chosen.includes(item.name)} onChange={() => toggle(item.name)} type="checkbox" />
                      {item.name}
                      <small className="export-check__profile">({fmt(inScope(item, scope))} {SCOPE_COUNT[scope]})</small>
                    </label>
                  ))}
                </div>
              )}

              <h3>Imagens</h3>
              <div className="export-formats process-scopes">
                {SCOPES.map((item) => (
                  <label className={scope === item.value ? 'export-format export-format--active' : 'export-format'} key={item.value}>
                    <input checked={scope === item.value} name="scope" onChange={() => setScope(item.value)} type="radio" />
                    <strong>{item.label}</strong>
                    <span>{item.hint}</span>
                  </label>
                ))}
              </div>

              <button aria-expanded={advanced} className="process-advanced" onClick={() => setAdvanced(!advanced)} type="button">
                <span aria-hidden="true" className="process-advanced__arrow">{advanced ? '▾' : '▸'}</span>
                Opções avançadas
                {!advanced && <small>tarefa {task || model.default_task} · score mínimo {minScore}</small>}
              </button>
              {advanced && (
                <div className="profile-form-grid">
                  <label className="form-field">
                    <span>Tarefa</span>
                    <input list="process-tasks" onChange={(event) => setTask(event.target.value)} placeholder={model.default_task} value={task} />
                    <datalist id="process-tasks">
                      {tasks.map((name) => <option key={name} value={name} />)}
                    </datalist>
                    <small className="profile-name-hint">Onde as caixas vão. Criada se não existir, com as labels do modelo.</small>
                  </label>
                  <label className="form-field">
                    <span>Score mínimo</span>
                    <input inputMode="decimal" max={1} min={0} onChange={(event) => setMinScore(event.target.value)} step={0.05} type="number" value={minScore} />
                    <small className="profile-name-hint">{model.score_help}</small>
                  </label>
                </div>
              )}

              <div className="export-actions">
                <button className="button button--primary" disabled={!!blocked || starting} onClick={() => { start() }} type="button">
                  {starting ? 'Iniciando...' : 'Processar'}
                </button>
                <span className="profile-name-hint">
                  {blocked || `${fmt(imageCount)} imagem(ns) ${SCOPE_TEXT[scope]} em ${fmt(chosen.length)} pacote(s). Caixas que vieram do CVAT ou foram editadas ficam como estão.`}
                </span>
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}

// O que está rodando agora, em cima do cartão.
function CurrentRun({ run, label, onCancel }: { run: ProcessingRun; label: string; onCancel: () => void }) {
  const percent = run.total > 0 ? Math.round((run.done / run.total) * 100) : 0
  return (
    <div className="create-card settings-card process-current">
      <div className="process-current__head">
        <div>
          <h2>Processando com {label}</h2>
          <p className="profile-name-hint">
            {fmt(run.done)} de {fmt(run.total)} imagem(ns) · {fmt(run.boxes)} caixa(s) · {duration(run)}
          </p>
        </div>
        <button className="button button--danger button--sm" onClick={onCancel} type="button">Cancelar</button>
      </div>
      <span className="progress-line process-current__bar"><span style={{ width: `${percent}%` }} /></span>
    </div>
  )
}

function History({ runs, modelLabel }: { runs: ProcessingRun[]; modelLabel: (key: string) => string }) {
  const [open, setOpen] = useState<string | null>(null)
  return (
    <div className="create-card settings-card">
      <h2>Histórico</h2>
      {runs.length === 0 ? (
        <p className="profile-name-hint">Nenhum processamento ainda.</p>
      ) : (
        <>
          <p className="profile-name-hint">Clique num processamento para ver os detalhes.</p>
          <table className="process-table">
            <thead>
              <tr>
                <th>Início</th>
                <th>Modelo</th>
                <th>Pacotes</th>
                <th>Status</th>
                <th>Resultado</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <RunRow key={run.id} label={modelLabel(run.model)} onToggle={() => setOpen(open === run.id ? null : run.id)} open={open === run.id} run={run} />
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  )
}

function RunRow({ run, label, open, onToggle }: { run: ProcessingRun; label: string; open: boolean; onToggle: () => void }) {
  const packages = run.packages.length > 2 ? `${run.packages.slice(0, 2).join(', ')} +${run.packages.length - 2}` : run.packages.join(', ')
  return (
    <>
      <tr aria-expanded={open} className={open ? 'process-row process-row--open' : 'process-row'} onClick={onToggle}>
        <td>{formatDate(run.started_at)}</td>
        <td>{label}</td>
        <td title={run.packages.join(', ')}>{packages}</td>
        <td><span className={`process-status process-status--${run.status}`}>{STATUS_TEXT[run.status]}</span></td>
        <td>
          {fmt(run.boxes)} caixa(s) em {fmt(run.done)}/{fmt(run.total)}
          {run.error_count > 0 && <span className="process-bad"> · {fmt(run.error_count)} erro(s)</span>}
        </td>
      </tr>
      {open && (
        <tr className="process-detail">
          <td colSpan={5}>
            <p>
              Imagens {SCOPE_TEXT[run.scope]} · tarefa <Link to={`/tarefas/${encodeURIComponent(run.task)}`}>{run.task}</Link> · score mínimo {run.min_score} · por {run.started_by} · {duration(run)}
            </p>
            <p>
              Pacotes: {run.packages.map((name, i) => (
                <span key={name}>{i > 0 && ', '}<Link to={`/pacotes/${encodeURIComponent(name)}`}>{name}</Link></span>
              ))}
            </p>
            <p>
              {fmt(run.written)} imagem(ns) gravada(s) na tarefa, com {fmt(run.boxes)} caixa(s)
              {run.skipped > 0 && ` · ${fmt(run.skipped)} pulada(s): as caixas vieram do CVAT ou foram editadas`}
            </p>
            {run.message && <p className="form-error">{run.message}</p>}
            {run.status === 'interrupted' && <p>O servidor reiniciou no meio. Rodar de novo é seguro: só as caixas do próprio modelo são regravadas.</p>}
            {run.errors && run.errors.length > 0 && (
              <ul className="process-errors">
                {run.errors.map((item) => <li key={item.sha1}><code>{item.sha1.slice(0, 12)}</code> {item.msg}</li>)}
                {run.error_count > run.errors.length && <li>e mais {fmt(run.error_count - run.errors.length)}</li>}
              </ul>
            )}
          </td>
        </tr>
      )}
    </>
  )
}
