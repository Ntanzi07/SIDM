import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  createVersion,
  deleteDataset,
  getAnnotationProfile,
  getDataset,
  getItemDiff,
  getVersionDiff,
  imageFileUrl,
  listVersions,
  type Dataset,
  type DiffRef,
  type ItemDiff,
  type Version,
  type VersionDiff,
} from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import { fieldLabel } from '../../utils/annotationForm'
import { facetsOf, fmt, valuesOf } from '../../utils/stats'
import { CountCard, LabelCard } from '../../components/Stats/StatsCards'
import { formatWhen, keyInfo, valueText } from './format'
import './Versions.css'

type Tab = 'summary' | 'diff'

// Um dataset: a pesquisa salva, as versões (da mais nova para a mais velha),
// o botão de criar a próxima e, da versão escolhida, o resumo e o que mudou.
export function DatasetPage() {
  const { name = '' } = useParams()
  const navigate = useNavigate()
  const [dataset, setDataset] = useState<Dataset | null>(null)
  const [versions, setVersions] = useState<Version[] | null>(null)
  const [profile, setProfile] = useState<AnnotationProfile | undefined>()
  const [chosen, setChosen] = useState(0) // 0 = a mais nova
  const [tab, setTab] = useState<Tab>('summary')
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    Promise.all([getDataset(name), listVersions(name)])
      .then(([item, list]) => {
        if (!active) return
        setDataset(item)
        setVersions(list)
        getAnnotationProfile(item.profile)
          .then((result) => { if (active) setProfile(result) })
          .catch(() => { /* perfil apagado: o resumo mostra só pacotes e labels */ })
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar o dataset.')
      })
    return () => { active = false }
  }, [name])

  const version = versions?.find((item) => item.number === chosen) ?? versions?.[0]

  function created(item: Version) {
    setVersions((current) => [item, ...(current ?? [])])
    setChosen(item.number)
    setTab(item.number > 1 ? 'diff' : 'summary')
  }

  return (
    <section className="versions-page">
      <button className="back-button" onClick={() => navigate('/versoes')} type="button">← Versões</button>
      {error && <p className="form-error">{error}</p>}
      {dataset && versions && (
        <>
          <div className="page-heading">
            <div>
              <span className="eyebrow">Perfil {dataset.profile}</span>
              <h1>{dataset.name}</h1>
              {dataset.description && <p>{dataset.description}</p>}
            </div>
            {versions.length === 0 && <DeleteDataset name={dataset.name} />}
          </div>

          <QuerySummary dataset={dataset} profile={profile} />

          <div className="versions-layout">
            <aside className="versions-side">
              <NewVersion dataset={dataset.name} onCreated={created} />
              <ol className="version-timeline">
                {versions.map((item, index) => {
                  const previous = versions[index + 1]
                  const delta = previous ? item.summary.images.total - previous.summary.images.total : 0
                  const current = item.number === version?.number
                  return (
                    <li key={item.id}>
                      <button
                        aria-current={current ? 'true' : undefined}
                        className={current ? 'version-item version-item--on' : 'version-item'}
                        onClick={() => setChosen(item.number)}
                        type="button"
                      >
                        <span className="version-badge">v{item.number}</span>
                        <span className="version-item__body">
                          <strong>{item.message}</strong>
                          <small>{item.created_by} · {formatWhen(item.created_at)}</small>
                          <small>
                            {fmt(item.summary.images.total)} imagem(ns)
                            {delta !== 0 && <em className={delta > 0 ? 'delta delta--up' : 'delta delta--down'}> {delta > 0 ? '+' : '−'}{fmt(Math.abs(delta))}</em>}
                          </small>
                        </span>
                      </button>
                    </li>
                  )
                })}
              </ol>
              {versions.length === 0 && <p className="versions-hint">Nenhuma versão ainda. Escreva o que esta versão tem e crie a primeira.</p>}
            </aside>

            {version && (
              <div className="version-panel" key={version.id}>
                <div className="version-panel__head">
                  <div>
                    <span className="eyebrow">Versão {version.number}</span>
                    <h2>{version.message}</h2>
                    <p className="versions-hint">
                      Criada por {version.created_by} em {formatWhen(version.created_at)}. Tarefas: {version.tasks.length > 0 ? version.tasks.join(', ') : 'nenhuma'}.
                    </p>
                  </div>
                  <div className="version-tabs" role="tablist">
                    <button aria-selected={tab === 'summary'} className={tab === 'summary' ? 'version-tab version-tab--on' : 'version-tab'} onClick={() => setTab('summary')} role="tab" type="button">Resumo</button>
                    <button aria-selected={tab === 'diff'} className={tab === 'diff' ? 'version-tab version-tab--on' : 'version-tab'} onClick={() => setTab('diff')} role="tab" type="button">O que mudou</button>
                  </div>
                </div>
                {tab === 'summary'
                  ? <VersionSummary profile={profile} version={version} />
                  : <DiffView dataset={dataset.name} key={version.number} profile={profile} to={version.number} versions={versions} />}
              </div>
            )}
          </div>
        </>
      )}
    </section>
  )
}

// A pesquisa que o dataset guarda, em uma linha de etiquetas.
function QuerySummary({ dataset, profile }: { dataset: Dataset; profile?: AnnotationProfile }) {
  const { packages, filters, tasks } = dataset.query
  return (
    <div className="query-line">
      <span className="query-line__group">
        <span className="eyebrow">Pacotes</span>
        {packages.length === 0 ? <span className="query-tag">todos do perfil</span> : packages.map((item) => <span className="query-tag" key={item}>{item}</span>)}
      </span>
      <span className="query-line__group">
        <span className="eyebrow">Filtros</span>
        {filters.length === 0 ? <span className="query-tag">nenhum</span> : filters.map((item) => {
          const info = keyInfo(profile, item.key)
          return <span className="query-tag" key={item.key}>{info.label} = {valueText(info.field, item.value)}</span>
        })}
      </span>
      <span className="query-line__group">
        <span className="eyebrow">Tarefas</span>
        {tasks.length === 0 ? <span className="query-tag">todas</span> : tasks.map((item) => <span className="query-tag" key={item}>{item}</span>)}
      </span>
      <span className="query-tag query-tag--fixed">só verificadas</span>
    </div>
  )
}

function NewVersion({ dataset, onCreated }: { dataset: string; onCreated: (version: Version) => void }) {
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      onCreated(await createVersion(dataset, message))
      setMessage('')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível criar a versão.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="new-version" onSubmit={(event) => void submit(event)}>
      <label htmlFor="version-message">Nova versão</label>
      <textarea
        id="version-message"
        onChange={(event) => setMessage(event.target.value)}
        placeholder="O que mudou desde a última? Ex.: corrigi a cor de 30 motos"
        rows={2}
        value={message}
      />
      {error && <p className="form-error">{error}</p>}
      <button className="button button--primary" disabled={busy || !message.trim()} type="submit">
        {busy ? 'Congelando as imagens...' : 'Criar versão'}
      </button>
    </form>
  )
}

function DeleteDataset({ name }: { name: string }) {
  const navigate = useNavigate()
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState('')

  async function remove() {
    try {
      await deleteDataset(name)
      navigate('/versoes')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar.')
    }
  }

  return (
    <div className="delete-dataset">
      {error && <p className="form-error">{error}</p>}
      {confirming ? (
        <>
          <button className="button button--danger button--sm" onClick={() => void remove()} type="button">Apagar de vez</button>
          <button className="button button--secondary button--sm" onClick={() => setConfirming(false)} type="button">Cancelar</button>
        </>
      ) : (
        <button className="button button--secondary button--sm" onClick={() => setConfirming(true)} title="Só sem versões" type="button">Apagar dataset</button>
      )}
    </div>
  )
}

// O resumo congelado na versão: as mesmas contas da Pesquisa.
function VersionSummary({ version, profile }: { version: Version; profile?: AnnotationProfile }) {
  const stats = version.summary.images
  const facets = useMemo(() => (profile ? facetsOf(profile) : []), [profile])
  return (
    <div className="version-summary">
      <div className="search-total">
        <strong>{fmt(stats.total)}</strong>
        <span>imagem(ns) verificada(s) nesta versão</span>
      </div>
      {version.summary.labels.length > 0 && (
        <>
          <h2 className="search-section">Labels do CVAT</h2>
          <div className="search-grid">
            {version.summary.labels.map((task) => <LabelCard key={task.task} searchImages={stats.total} task={task} />)}
          </div>
          <h2 className="search-section">Campos da anotação</h2>
        </>
      )}
      <div className="search-grid">
        <CountCard
          of={stats.total}
          rows={Object.keys(stats.packages).sort().map((item) => ({ value: item, label: item, count: stats.packages[item] }))}
          title="Por pacote"
        />
        {Object.entries(stats.objects).map(([type, item]) => (
          <div className="search-card" key={type}>
            <h2>{fieldLabel(profile?.objects.find((object) => object.key === type) ?? { key: type })}</h2>
            <p className="search-card__big">{fmt(item.count)} <span>objeto(s) em {fmt(item.images)} imagem(ns)</span></p>
          </div>
        ))}
        {facets.map((facet) => {
          const counts = facet.objectType ? stats.objects[facet.objectType]?.fields[facet.field.key] : stats.fields[facet.field.key]
          const missing = facet.objectType ? stats.objects[facet.objectType]?.missing[facet.field.key] ?? 0 : stats.missing[facet.field.key] ?? 0
          const of = facet.objectType ? stats.objects[facet.objectType]?.count ?? 0 : stats.total
          const known = valuesOf(facet.field)
          const extra = Object.keys(counts ?? {}).filter((value) => !known.some((item) => item.value === value))
          const rows = [...known, ...extra.map((value) => ({ value, label: value }))].map((item) => ({ ...item, count: counts?.[item.value] ?? 0 }))
          return <CountCard key={facet.key} missing={missing} of={of} rows={rows} title={facet.title} />
        })}
      </div>
    </div>
  )
}

// O diff: a versão escolhida contra outra (a anterior, de início).
function DiffView({ dataset, to, versions, profile }: { dataset: string; to: number; versions: Version[]; profile?: AnnotationProfile }) {
  const [from, setFrom] = useState(to - 1) // 0 = nenhuma (a v1: tudo entrou)
  const others = versions.filter((item) => item.number !== to)
  return (
    <div className="version-diff">
      <div className="diff-pick">
        <label className="search-select">
          <span>Comparar a v{to} com</span>
          <select onChange={(event) => setFrom(Number(event.target.value))} value={from}>
            {to === 1 && <option value={0}>nada (a primeira versão)</option>}
            {others.map((item) => <option key={item.number} value={item.number}>v{item.number} · {item.message}</option>)}
          </select>
        </label>
        {from > to && <p className="versions-hint">Comparando com uma versão mais nova: “entrou” e “saiu” ficam ao contrário.</p>}
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
              <tr><th>Campo</th><th>{fromLabel}</th><th>v{to}</th></tr>
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
