import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  createVersion,
  deleteDataset,
  getAnnotationProfile,
  getDataset,
  listVersions,
  type Dataset,
  type Version,
} from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import { fmt } from '../../utils/stats'
import { formatWhen, keyInfo, valueText } from './format'
import { spanText } from '../../components/Charts/palette'
import { DatasetTimeline, VersionPanel, type PanelTab } from './VersionParts'
import { useToastState } from '../../hooks/useToast'
import { useIsAdmin } from '../../services/user'
import './Versions.css'

// Um dataset: a pesquisa salva, as versões (da mais nova para a mais velha),
// o botão de criar a próxima e, da versão escolhida, o resumo e o que mudou.
export function DatasetPage() {
  const { name = '' } = useParams()
  const navigate = useNavigate()
  const [dataset, setDataset] = useState<Dataset | null>(null)
  const [versions, setVersions] = useState<Version[] | null>(null)
  const [profile, setProfile] = useState<AnnotationProfile | undefined>()
  const [chosen, setChosen] = useState(0) // 0 = a mais nova
  const [tab, setTab] = useState<PanelTab>('summary')
  const [, setError] = useToastState('error')

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
  }, [name, setError])

  const version = versions?.find((item) => item.number === chosen) ?? versions?.[0]

  function created(item: Version) {
    setVersions((current) => [item, ...(current ?? [])])
    setChosen(item.number)
    setTab(item.number > 1 ? 'diff' : 'summary')
  }

  return (
    <section className="versions-page">
      <button className="back-button" onClick={() => navigate(dataset ? `/dashboard/${encodeURIComponent(dataset.profile)}?dataset=${encodeURIComponent(dataset.name)}` : '/dashboard')} type="button">← Dashboard</button>
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

          {versions.length > 0 && <DatasetTimeline dataset={dataset.name} onSelect={setChosen} selected={version?.number} versions={versions} />}

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
                          {previous && <small className="version-item__gap">{spanText(new Date(item.created_at).getTime() - new Date(previous.created_at).getTime())} depois da v{previous.number}</small>}
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
              <VersionPanel dataset={dataset.name} key={version.id} onTab={setTab} profile={profile} tab={tab} version={version} versions={versions} />
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
  const [, setError] = useToastState('error')

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
      <button className="button button--primary" disabled={busy || !message.trim()} type="submit">
        {busy ? 'Congelando as imagens...' : 'Criar versão'}
      </button>
    </form>
  )
}

function DeleteDataset({ name }: { name: string }) {
  const navigate = useNavigate()
  const admin = useIsAdmin() // apagar é só de admin
  const [confirming, setConfirming] = useState(false)
  const [, setError] = useToastState('error')

  async function remove() {
    try {
      await deleteDataset(name)
      navigate('/dashboard')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar.')
    }
  }

  if (!admin) return null
  return (
    <div className="delete-dataset">
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

