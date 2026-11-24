import { useEffect, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import {
  createVersion,
  downloadVersionExport,
  getItemDiff,
  getVersionDiff,
  getVersionDiffCounts,
  imageFileUrl,
  listExportProfiles,
  previewVersionExport,
  type DiffRef,
  type ExportPreview,
  type ExportProfile,
  type ItemDiff,
  type Version,
  type VersionDiff,
} from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import { fmt } from '../../utils/stats'
import { SearchPage } from '../Search/SearchPage'
import { Timeline, type TimelinePoint } from '../../components/Charts/Timeline'
import { spanText } from '../../components/Charts/palette'
import { formatWhen, keyInfo, valueText } from './format'
import './Versions.css'

// As partes de uma versão usadas na tela do dataset e no Dashboard: a linha
// do tempo do dataset e o painel da versão (resumo e o que mudou).

export type PanelTab = 'summary' | 'diff'

export function DatasetTimeline({ dataset, versions, current, selected, onSelect, actions }: {
  dataset: string
  versions: Version[]
  current?: Version // o agora (number 0), sem gravar: o último ponto, tracejado
  selected?: number
  onSelect: (number: number) => void
  actions?: ReactNode // botões no cabeçalho (ex.: nova versão)
}) {
  // o que mudou em cada versão (contra a anterior)
  const [changes, setChanges] = useState<Record<number, { added: number; removed: number; changed: number }>>({})
  const numbers = versions.map((item) => item.number).join(',')
  useEffect(() => {
    if (!numbers) return
    let active = true
    for (const n of numbers.split(',').map(Number)) {
      getVersionDiffCounts(dataset, n - 1, n)
        .then((counts) => { if (active) setChanges((current) => ({ ...current, [n]: counts })) })
        .catch(() => { /* a linha do tempo fica sem as barras dessa versão */ })
    }
    return () => { active = false }
  }, [dataset, numbers])

  // o que mudou da mais nova até agora
  const [nowChanges, setNowChanges] = useState<{ added: number; removed: number; changed: number } | undefined>()
  const hasNow = Boolean(current)
  useEffect(() => {
    if (!hasNow) return
    let active = true
    getVersionDiffCounts(dataset, 0, 0)
      .then((counts) => { if (active) setNowChanges(counts) })
      .catch(() => { /* sem as barras do agora */ })
    return () => { active = false }
  }, [dataset, hasNow, numbers])

  const points: TimelinePoint[] = versions.map((item) => ({
    number: item.number,
    date: item.created_at,
    message: item.message,
    by: item.created_by,
    images: item.summary.images.total,
    changes: changes[item.number],
  }))
  if (current) {
    points.push({ number: 0, date: current.created_at, message: '', by: 'estado atual', images: current.summary.images.total, changes: nowChanges })
  }
  const newest = versions[0]
  const oldest = versions[versions.length - 1]

  return (
    <section className="version-timeline-card">
      <div className="version-timeline-card__head">
        <div>
          <h2>Linha do tempo · {dataset}</h2>
          <span>
            {versions.length} versão(ões)
            {versions.length > 1 && ` em ${spanText(new Date(newest.created_at).getTime() - new Date(oldest.created_at).getTime())}`}
            {newest && ` · última ${formatWhen(newest.created_at)}`}
            {newest && current && ` · ${spanText(new Date(current.created_at).getTime() - new Date(newest.created_at).getTime())} sem salvar`}
          </span>
        </div>
        {actions && <div className="version-timeline-card__actions">{actions}</div>}
      </div>
      <Timeline onSelect={onSelect} points={points} selected={selected} />
      <div className="timeline-legend">
        <span><i style={{ background: 'var(--accent)' }} />imagens na versão</span>
        <span><i style={{ background: 'var(--success)' }} />entraram</span>
        <span><i style={{ background: 'var(--warning)' }} />alteradas</span>
        <span><i style={{ background: 'var(--danger)' }} />saíram</span>
        {current && <span><i className="timeline-legend__now" />agora (não salvo)</span>}
        <span>· clique num ponto para ver os dados dele</span>
      </div>
    </section>
  )
}

export function VersionPanel({ dataset, version, versions, profile, tab, onTab, onSave }: {
  dataset: string
  version: Version // number 0 = o agora
  versions: Version[] // as gravadas
  profile?: AnnotationProfile
  tab: PanelTab
  onTab: (tab: PanelTab) => void
  onSave?: () => void // o agora: "Salvar como versão"
}) {
  const now = version.number === 0
  const [downloading, setDownloading] = useState(false)
  return (
    <div className="version-panel">
      <div className="version-panel__head">
        <div>
          <span className="eyebrow">{dataset} · {now ? 'agora, não salvo' : `versão ${version.number}`}</span>
          <h2>{now ? 'Estado atual' : version.message}</h2>
          <p className="versions-hint">
            {now
              ? `As verificadas que passam na pesquisa do dataset neste momento. Salve como versão para congelar. Tarefas: ${version.tasks.join(', ') || 'nenhuma'}.`
              : `Criada por ${version.created_by} em ${formatWhen(version.created_at)}. Tarefas: ${version.tasks.length > 0 ? version.tasks.join(', ') : 'nenhuma'}.`}
          </p>
          <div className="version-panel__actions">
            {now && onSave && (
              <button className="button button--primary button--sm" onClick={onSave} type="button">Salvar como versão</button>
            )}
            <button className="button button--secondary button--sm" onClick={() => setDownloading(true)} type="button">
              ↓ Baixar {now ? 'o estado atual' : `a v${version.number}`}
            </button>
          </div>
        </div>
        <div className="version-tabs" role="tablist">
          <button aria-selected={tab === 'summary'} className={tab === 'summary' ? 'version-tab version-tab--on' : 'version-tab'} onClick={() => onTab('summary')} role="tab" type="button">Dados</button>
          <button aria-selected={tab === 'diff'} className={tab === 'diff' ? 'version-tab version-tab--on' : 'version-tab'} onClick={() => onTab('diff')} role="tab" type="button">O que mudou</button>
        </div>
      </div>
      {tab === 'summary'
        ? (
          <SearchPage
            profileName={version.profile}
            version={{ dataset, number: version.number }}
          />
        )
        : <DiffView dataset={dataset} key={version.number} profile={profile} to={version.number} versions={versions} />}
      {downloading && <DownloadVersionModal dataset={dataset} onClose={() => setDownloading(false)} version={version} />}
    </div>
  )
}

// Baixar uma versão como dataset de treino: o perfil de export escolhe o
// formato (COCO, YOLO, VOC, só imagens), a divisão e os nomes; as imagens e
// a anotação são as congeladas na versão.
function DownloadVersionModal({ dataset, version, onClose }: { dataset: string; version: Version; onClose: () => void }) {
  const now = version.number === 0
  const [profiles, setProfiles] = useState<ExportProfile[] | null>(null)
  const [chosen, setChosen] = useState('')
  const [picked, setPicked] = useState<string[] | null>(null) // null = todas as tarefas da versão
  const [result, setResult] = useState<{ key: string; preview?: ExportPreview; error?: string } | null>(null)
  const [started, setStarted] = useState(false)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  useEffect(() => {
    listExportProfiles()
      .then(setProfiles)
      .catch(() => setProfiles([]))
  }, [])

  // Um perfil salvo só serve para o perfil de anotação dele; os padrões, para todos.
  const usable = (profiles ?? []).filter((item) => item.builtin || (item.annotation_profile || 'veiculos') === version.profile)
  const profile = usable.find((item) => item.name === chosen) ?? usable[0]
  const withBoxes = profile ? profile.format !== 'images' : true
  const tasks = picked ?? version.tasks
  const body = profile
    ? { profile: profile.name, tasks: withBoxes ? tasks : undefined, version: { dataset, number: version.number } }
    : null
  const key = body ? JSON.stringify(body) : ''

  useEffect(() => {
    if (!body || (withBoxes && body.tasks?.length === 0)) return
    let alive = true
    previewVersionExport(body)
      .then((preview) => { if (alive) setResult({ key, preview }) })
      .catch((error: unknown) => { if (alive) setResult({ key, error: error instanceof Error ? error.message : 'Não foi possível montar a prévia.' }) })
    return () => { alive = false }
    // body muda junto com key
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  const noTasks = withBoxes && tasks.length === 0
  const current = result?.key === key ? result : null
  const loading = !!body && !noTasks && !current
  const preview = current?.preview

  function toggle(task: string) {
    setPicked(tasks.includes(task) ? tasks.filter((item) => item !== task) : version.tasks.filter((item) => item === task || tasks.includes(item)))
  }

  function download() {
    if (!body) return
    downloadVersionExport(body)
    setStarted(true)
  }

  const zipName = profile ? `${dataset}-${now ? 'atual-…' : `v${version.number}`}-${profile.name}.zip` : ''

  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={onClose} role="dialog">
      <div className="modal-dialog modal-dialog--wide" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Baixar {now ? 'o estado atual' : `a v${version.number}`} de {dataset}</h2>
          <button aria-label="Fechar" className="modal-close" onClick={onClose} type="button">✕</button>
        </div>
        <div className="modal-body download-version">
          <p className="versions-hint">
            {now
              ? 'Sai como está agora (sem salvar como versão). Para poder baixar de novo igual depois, salve uma versão antes.'
              : `Sai exatamente como estava ao criar a v${version.number} (${formatWhen(version.created_at)}): mudanças de anotação depois não entram.`}
          </p>

          {profiles === null ? <p className="versions-hint">Carregando os perfis de export...</p> : (
            <>
              <label className="search-select">
                <span>Formato (perfil de export)</span>
                <select onChange={(event) => setChosen(event.target.value)} value={profile?.name ?? ''}>
                  {usable.map((item) => (
                    <option key={item.name} value={item.name}>
                      {item.name} · {FORMAT_LABEL[item.format] ?? item.format}{item.builtin ? '' : ' (salvo)'}
                    </option>
                  ))}
                </select>
              </label>
              <p className="versions-hint">Do perfil valem o formato, a divisão (train/valid/test), as pastas, os nomes e as labels. Os pacotes e os filtros dele não: as imagens são as da versão.</p>

              {withBoxes && (
                <fieldset className="download-version__tasks">
                  <legend>Caixas das tarefas</legend>
                  {version.tasks.length === 0 && <p className="versions-hint">Esta versão não tem tarefas: só dá para baixar as imagens.</p>}
                  {version.tasks.map((task) => (
                    <label key={task}>
                      <input checked={tasks.includes(task)} onChange={() => toggle(task)} type="checkbox" />
                      {task}
                    </label>
                  ))}
                </fieldset>
              )}

              <div className="download-version__preview">
                {noTasks && <p className="versions-hint">Escolha ao menos uma tarefa.</p>}
                {loading && <p className="versions-hint">Montando a prévia...</p>}
                {current?.error && <p className="form-error">{current.error}</p>}
                {preview && <PreviewSummary preview={preview} />}
              </div>
            </>
          )}
          {started && (
            <p className="download-version__started">
              O download começou: o navegador salva <code>{zipName}</code>. Com muitas imagens, pode levar um tempo para aparecer.
            </p>
          )}
        </div>
        <div className="modal-actions">
          <button className="button button--secondary" onClick={onClose} type="button">{started ? 'Fechar' : 'Cancelar'}</button>
          <button className="button button--primary" disabled={!preview || preview.total === 0} onClick={download} type="button">
            ↓ Baixar zip{preview ? ` (${fmt(preview.total)} imagens)` : ''}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}

const FORMAT_LABEL: Record<string, string> = { coco: 'COCO', yolo: 'YOLO', voc: 'Pascal VOC', images: 'só imagens' }

function PreviewSummary({ preview }: { preview: ExportPreview }) {
  const skipped = preview.skipped.not_annotated + preview.skipped.missing_tasks + preview.skipped.without_labels
  const classes = Object.entries(preview.boxes).sort((a, b) => b[1] - a[1])
  return (
    <div className="download-version__summary">
      <div className="download-version__total">
        <strong>{fmt(preview.total)}</strong>
        <span>de {fmt(preview.version?.images ?? preview.total)} imagens da versão entram</span>
      </div>
      <div className="download-version__splits">
        {Object.entries(preview.images).map(([split, count]) => (
          <span key={split}><strong>{fmt(count)}</strong> {split}</span>
        ))}
        {preview.negatives > 0 && <span><strong>{fmt(preview.negatives)}</strong> sem objetos (negativas)</span>}
      </div>
      {classes.length > 0 && (
        <div className="download-version__classes">
          {classes.map(([label, count]) => <span key={label}>{label} <strong>{fmt(count)}</strong></span>)}
        </div>
      )}
      {skipped > 0 && (
        <p className="versions-hint">
          Ficam de fora {fmt(skipped)}:
          {preview.skipped.not_annotated > 0 && ` ${fmt(preview.skipped.not_annotated)} sem anotação nas tarefas escolhidas;`}
          {preview.skipped.missing_tasks > 0 && ` ${fmt(preview.skipped.missing_tasks)} anotadas em só parte das tarefas;`}
          {preview.skipped.without_labels > 0 && ` ${fmt(preview.skipped.without_labels)} só com labels que o perfil não leva.`}
        </p>
      )}
      {preview.examples.length > 0 && (
        <p className="versions-hint">Nomes: {preview.examples.map((name) => <code key={name}>{name}</code>)}</p>
      )}
    </div>
  )
}

// O diff: a versão escolhida contra outra (a anterior, de início).
function DiffView({ dataset, to, versions, profile }: { dataset: string; to: number; versions: Version[]; profile?: AnnotationProfile }) {
  // o agora (0) começa contra a mais nova; uma versão, contra a anterior.
  // 0 em from = nada (tudo entrou).
  const [from, setFrom] = useState(to === 0 ? (versions[0]?.number ?? 0) : to - 1)
  const others = versions.filter((item) => item.number !== to)
  const toLabel = to === 0 ? 'o agora' : `a v${to}`
  return (
    <div className="version-diff">
      <div className="diff-pick">
        <label className="search-select">
          <span>Comparar {toLabel} com</span>
          <select onChange={(event) => setFrom(Number(event.target.value))} value={from}>
            {(to === 1 || (to === 0 && versions.length === 0)) && <option value={0}>nada (tudo entra)</option>}
            {others.map((item) => <option key={item.number} value={item.number}>v{item.number} · {item.message}</option>)}
          </select>
        </label>
        {to !== 0 && from > to && <p className="versions-hint">Comparando com uma versão mais nova: “entrou” e “saiu” ficam ao contrário.</p>}
      </div>
      <DiffResult dataset={dataset} from={from} key={from} profile={profile} to={to} />
    </div>
  )
}

type Group = 'added' | 'removed' | 'changed'

const GROUPS: Array<{ key: Group; label: string; sign: string }> = [
  { key: 'changed', label: 'Alteradas', sign: '~' },
  { key: 'added', label: 'Entraram', sign: '+' },
  { key: 'removed', label: 'Saíram', sign: '−' },
]

const DIFF_PAGE = 50

function DiffResult({ dataset, from, to, profile }: { dataset: string; from: number; to: number; profile?: AnnotationProfile }) {
  const [diff, setDiff] = useState<VersionDiff | null>(null)
  const [error, setError] = useState('')
  const [picked, setPicked] = useState<Group | ''>('') // '' = o primeiro grupo com imagens
  const [shown, setShown] = useState(DIFF_PAGE)
  const [open, setOpen] = useState('')

  useEffect(() => {
    let active = true
    getVersionDiff(dataset, from, to)
      .then((result) => { if (active) setDiff(result) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível comparar.')
      })
    return () => { active = false }
  }, [dataset, from, to])

  if (error) return <p className="form-error">{error}</p>
  if (!diff) return <p className="versions-hint">Comparando...</p>

  const group = picked || GROUPS.find((item) => diff[item.key].length > 0)?.key || 'changed'
  const list: DiffRef[] = diff[group]
  const nothing = diff.added.length + diff.removed.length + diff.changed.length === 0

  return (
    <>
      <div className="diff-counts">
        {GROUPS.map((item) => (
          <button
            className={['diff-count', `diff-count--${item.key}`, group === item.key ? 'diff-count--on' : ''].join(' ')}
            disabled={diff[item.key].length === 0}
            key={item.key}
            onClick={() => { setPicked(item.key); setShown(DIFF_PAGE); setOpen('') }}
            type="button"
          >
            <strong>{item.sign}{fmt(diff[item.key].length)}</strong>
            <span>{item.label}</span>
          </button>
        ))}
        <div className="diff-count diff-count--same">
          <strong>{fmt(diff.unchanged)}</strong>
          <span>Iguais</span>
        </div>
      </div>

      {nothing ? (
        <p className="versions-hint">Nenhuma mudança: as duas versões têm as mesmas imagens com a mesma anotação.</p>
      ) : (
        <ul className="diff-list">
          {list.slice(0, shown).map((item) => (
            <li key={item.sha1}>
              <button
                aria-expanded={open === item.sha1}
                className={open === item.sha1 ? 'diff-row diff-row--open' : 'diff-row'}
                onClick={() => setOpen(open === item.sha1 ? '' : item.sha1)}
                type="button"
              >
                <span className={`diff-dot diff-dot--${group}`} />
                <span className="diff-row__name" title={item.file_name}>{item.original_name || item.file_name || item.sha1}</span>
                <span className="diff-row__package">{item.package}</span>
              </button>
              {open === item.sha1 && <ItemDetail dataset={dataset} from={from} profile={profile} sha1={item.sha1} to={to} />}
            </li>
          ))}
          {list.length > shown && (
            <li>
              <button className="button button--secondary button--sm diff-more" onClick={() => setShown(shown + DIFF_PAGE)} type="button">
                Mostrar mais ({fmt(shown)} de {fmt(list.length)})
              </button>
            </li>
          )}
        </ul>
      )}
    </>
  )
}

// O que mudou numa imagem: os campos, os objetos e as caixas por tarefa.
function ItemDetail({ dataset, from, to, sha1, profile }: { dataset: string; from: number; to: number; sha1: string; profile?: AnnotationProfile }) {
  const [item, setItem] = useState<ItemDiff | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    getItemDiff(dataset, from, to, sha1)
      .then((result) => { if (active) setItem(result) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível abrir o detalhe.')
      })
    return () => { active = false }
  }, [dataset, from, to, sha1])

  if (error) return <p className="form-error">{error}</p>
  if (!item) return <p className="versions-hint diff-detail">Carregando...</p>

  const changes = [...item.fields, ...item.objects]
  const fromLabel = from > 0 ? `v${from}` : 'antes'
  return (
    <div className="diff-detail">
      <img alt="" className="diff-detail__image" loading="lazy" src={imageFileUrl(item.sha1)} />
      <div className="diff-detail__body">
        {changes.length > 0 && (
          <table className="diff-table">
            <thead>
              <tr><th>Campo</th><th>{fromLabel}</th><th>{to === 0 ? 'agora' : `v${to}`}</th></tr>
            </thead>
            <tbody>
              {changes.map((change) => {
                const info = keyInfo(profile, change.key)
                return (
                  <tr key={change.key}>
                    <td>{info.label}</td>
                    <td><Value text={valueText(info.field, change.from)} /></td>
                    <td><Value text={valueText(info.field, change.to)} /></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
        {item.tasks.length > 0 && (
          <ul className="diff-tasks">
            {item.tasks.map((task) => (
              <li key={task.task}>
                <strong>Tarefa {task.task}</strong>
                <span>{fmt(task.from)} → {fmt(task.to)} caixa(s)</span>
                {task.labels.map((label) => (
                  <span className="diff-label" key={label.label}>{label.label}: {fmt(label.from)} → {fmt(label.to)}</span>
                ))}
                {task.edited && <span className="diff-label">caixas movidas ou editadas (as mesmas quantidades)</span>}
              </li>
            ))}
          </ul>
        )}
        {changes.length === 0 && item.tasks.length === 0 && <p className="versions-hint">Sem diferença na anotação.</p>}
        <Link className="diff-detail__link" to={`/pacotes/${encodeURIComponent(item.package)}/verificadas?sha1=${item.sha1}`}>
          Ver a imagem como está hoje →
        </Link>
      </div>
    </div>
  )
}

function Value({ text }: { text: string | null }) {
  return text === null ? <span className="diff-missing">Não informado</span> : <>{text}</>
}

// Criar a próxima versão de um dataset, num pop-up (o Dashboard usa).
export function NewVersionModal({ dataset, onClose, onCreated }: { dataset: string; onClose: () => void; onCreated: (version: Version) => void }) {
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      onCreated(await createVersion(dataset, message.trim()))
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível criar a versão.')
      setBusy(false)
    }
  }

  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={() => { if (!busy) onClose() }} role="dialog">
      <div className="modal-dialog" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Nova versão de {dataset}</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy} onClick={onClose} type="button">✕</button>
        </div>
        <form onSubmit={(event) => void submit(event)}>
          <div className="modal-body new-version-modal">
            <p className="versions-hint">Congela agora as imagens verificadas que passam na pesquisa do dataset, com a anotação delas. A versão nunca muda depois.</p>
            <label htmlFor="new-version-message">O que mudou desde a última?</label>
            <textarea autoFocus id="new-version-message" onChange={(event) => setMessage(event.target.value)} placeholder="Ex.: corrigi a cor de 30 motos" rows={3} value={message} />
            {error && <p className="form-error">{error}</p>}
          </div>
          <div className="modal-actions">
            <button className="button button--secondary" disabled={busy} onClick={onClose} type="button">Cancelar</button>
            <button className="button button--primary" disabled={busy || !message.trim()} type="submit">{busy ? 'Congelando as imagens...' : 'Criar versão'}</button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  )
}
