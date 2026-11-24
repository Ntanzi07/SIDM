import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { deleteDataset, getAnnotationProfile, getCurrentVersion, listAnnotationProfiles, listDatasets, listPackagesOfProfile, listVersions, type Dataset, type Version } from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import { SearchPage } from '../Search/SearchPage'
import { DatasetTimeline, NewVersionModal, VersionPanel, type PanelTab } from '../Versions/VersionParts'
import { formatWhen } from '../Versions/format'
import { Sparkline } from '../../components/Charts/Sparkline'
import { fmt } from '../../utils/stats'
import { useIsAdmin } from '../../services/user'
import { PageLoader } from '../../components/PageLoader/PageLoader'
import './Dashboard.css'

// Dashboard: primeiro escolhe o perfil de anotação; depois, à esquerda, de
// onde vêm os dados (todas as imagens agora, ou uma versão de um dataset) e,
// à direita, os números e gráficos dessa fonte.
export function DashboardPage() {
  const { profile } = useParams()
  // state.fresh: quem acabou de criar um dataset pede a lista de novo (a
  // tela é montada outra vez em vez de reaproveitar a lista antiga)
  const fresh = (useLocation().state as { fresh?: number } | null)?.fresh ?? 0
  return profile ? <ProfileDashboard key={`${profile}#${fresh}`} profile={profile} /> : <ProfilePicker />
}

function ProfilePicker() {
  const [profiles, setProfiles] = useState<AnnotationProfile[] | null>(null)
  const [packages, setPackages] = useState<Record<string, number>>({})
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    listAnnotationProfiles()
      .then((items) => {
        if (!active) return
        setProfiles(items)
        // quantos pacotes usam cada perfil (um extra: sem ele os cartões aparecem igual)
        for (const item of items) {
          listPackagesOfProfile(item.name)
            .then((list) => { if (active) setPackages((current) => ({ ...current, [item.name]: list.length })) })
            .catch(() => { /* só informativo */ })
        }
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis.')
      })
    return () => { active = false }
  }, [])

  return (
    <section className="dashboard-page">
      <div className="page-heading">
        <div>
          <h1>Dashboard</h1>
          <p>Escolha um perfil de anotação para ver os números das imagens e as versões dos datasets dele.</p>
        </div>
      </div>
      {error && <p className="form-error">{error}</p>}
      {profiles === null && !error && <PageLoader label="Carregando os perfis" />}
      {profiles && (
        <div className="profile-grid">
          {profiles.map((item, index) => (
            <Link className="profile-card" key={item.name} style={{ animationDelay: `${Math.min(index, 10) * 45}ms` }} to={`/dashboard/${encodeURIComponent(item.name)}`}>
              <span className="eyebrow">Perfil</span>
              <h2>{item.name}</h2>
              {item.description && <p>{item.description}</p>}
              <dl>
                <div><dt>Campos</dt><dd>{item.fields.length}</dd></div>
                <div><dt>Objetos</dt><dd>{item.objects.length}</dd></div>
                <div><dt>Pacotes</dt><dd>{packages[item.name] ?? '—'}</dd></div>
              </dl>
            </Link>
          ))}
        </div>
      )}
    </section>
  )
}

// A fonte dos dados da direita: todas as imagens do perfil (agora, com os
// filtros), ou uma versão congelada de um dataset. Na URL:
// ?dataset=nome&v=3 (sem v: a mais nova).
function ProfileDashboard({ profile }: { profile: string }) {
  const navigate = useNavigate()
  const admin = useIsAdmin() // apagar é só de admin
  const [params, setParams] = useSearchParams()
  // ?novo=1: todas as imagens, para montar e salvar um dataset novo
  const creatingDataset = params.get('novo') === '1'
  // ?v=3: uma versão; sem v: o agora (o estado atual, não salvo)
  const versionNumber = Number(params.get('v')) || 0
  const [datasets, setDatasets] = useState<Array<{ dataset: Dataset; versions: Version[] }> | null>(null)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0) // muda depois de criar uma versão
  const [creating, setCreating] = useState('') // o dataset do pop-up "Nova versão"
  const [deleting, setDeleting] = useState('') // o dataset do pop-up "Apagar"

  useEffect(() => {
    let active = true
    listDatasets()
      .then(async (items) => {
        const mine = items.filter((item) => item.profile === profile)
        const withVersions = await Promise.all(mine.map(async (dataset) => ({
          dataset,
          versions: dataset.latest ? await listVersions(dataset.name).catch(() => [] as Version[]) : [],
        })))
        if (active) setDatasets(withVersions)
      })
      .catch((requestError: unknown) => {
        if (!active) return
        setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os datasets.')
        setDatasets([])
      })
    return () => { active = false }
  }, [profile, reload])

  const pick = (dataset = '', v = 0) => {
    const next: Record<string, string> = {}
    if (dataset) next.dataset = dataset
    if (v) next.v = String(v)
    setParams(next, { replace: true })
  }

  // sem ?dataset: o primeiro do perfil
  const datasetName = params.get('dataset') ?? (creatingDataset ? '' : datasets?.[0]?.dataset.name ?? '')
  const chosen = datasets?.find((item) => item.dataset.name === datasetName)

  function created(version: Version) {
    setCreating('')
    setReload((n) => n + 1)
    pick(version.dataset, version.number)
  }

  return (
    <section className="dashboard-page">
      <button className="back-button" onClick={() => navigate('/dashboard')} type="button">← Perfis</button>
      <div className="page-heading dashboard-heading">
        <div>
          <span className="eyebrow">Dashboard</span>
          <h1>{profile}</h1>
        </div>
      </div>

      <div className="dashboard-layout">
        <aside className="dashboard-side">
          <button
            className="button button--primary dashboard-side__save"
            disabled={!chosen}
            onClick={() => { if (chosen) setCreating(chosen.dataset.name) }}
            title={chosen ? `Congelar o estado atual de ${chosen.dataset.name}` : 'Escolha um dataset'}
            type="button"
          >
            Salvar versão
          </button>
          {chosen && <small className="dashboard-side__note">o estado atual de <strong>{chosen.dataset.name}</strong> vira a v{(chosen.versions[0]?.number ?? 0) + 1}</small>}

          <span className="eyebrow dashboard-side__group">Versões</span>
          {error && <p className="form-error">{error}</p>}
          {datasets === null && <PageLoader inline label="Carregando as versões" />}
          {datasets?.length === 0 && !error && (
            <p className="dashboard-hint">Nenhum dataset ainda. Monte a pesquisa à direita e use “Salvar como dataset”.</p>
          )}
          {datasets?.map(({ dataset, versions }) => {
            const open = dataset.name === datasetName
            return (
              <div className={open ? 'source-group source-group--open' : 'source-group'} key={dataset.name}>
                <button className={open ? 'source-item source-item--on' : 'source-item'} onClick={() => pick(dataset.name)} type="button">
                  <span className="source-item__row">
                    <strong>{dataset.name}</strong>
                    {versions.length > 1 && <Sparkline height={20} values={[...versions].reverse().map((v) => v.summary.images.total)} width={56} />}
                  </span>
                  <small>
                    {versions.length > 0 ? `${versions.length} versão(ões) · v${versions[0].number} ${formatWhen(versions[0].created_at)}` : 'sem versões'}
                  </small>
                </button>
                {admin && <button aria-label={`Apagar o dataset ${dataset.name}`} className="source-delete" onClick={() => setDeleting(dataset.name)} title="Apagar este dataset" type="button">✕</button>}
                {open && (
                  <ol className="source-versions">
                    <li>
                      <button className={!versionNumber ? 'source-version source-version--on source-version--now' : 'source-version source-version--now'} onClick={() => pick(dataset.name)} type="button">
                        <span className="version-badge">agora</span>
                        <span>estado atual, não salvo</span>
                        <small />
                      </button>
                    </li>
                    {versions.map((v) => {
                      const on = versionNumber === v.number
                      return (
                        <li key={v.id}>
                          <button className={on ? 'source-version source-version--on' : 'source-version'} onClick={() => pick(dataset.name, v.number)} title={v.message} type="button">
                            <span className="version-badge">v{v.number}</span>
                            <span>{v.message}</span>
                            <small>{fmt(v.summary.images.total)}</small>
                          </button>
                        </li>
                      )
                    })}
                  </ol>
                )}
              </div>
            )
          })}
          {datasets !== null && datasets.length > 0 && (
            <button className={creatingDataset ? 'source-new source-new--on' : 'source-new'} onClick={() => setParams({ novo: '1' }, { replace: true })} type="button">
              + Novo dataset
            </button>
          )}
        </aside>

        <div className="dashboard-main">
          {datasets !== null && !datasetName && <SearchPage profileName={profile} />}
          {datasetName && datasets !== null && !chosen && <p className="form-error">O dataset {datasetName} não existe neste perfil.</p>}
          {chosen && (
            <VersionSource
              dataset={chosen.dataset}
              key={`${chosen.dataset.name}#${reload}`}
              number={versionNumber}
              onPick={(n) => pick(chosen.dataset.name, n)}
              versions={chosen.versions}
            />
          )}
        </div>
      </div>
      {creating && <NewVersionModal dataset={creating} onClose={() => setCreating('')} onCreated={created} />}
      {deleting && (
        <DeleteDatasetModal
          name={deleting}
          versions={datasets?.find((item) => item.dataset.name === deleting)?.versions.length ?? 0}
          onClose={() => setDeleting('')}
          onDeleted={() => {
            setDeleting('')
            if (deleting === datasetName) setParams({}, { replace: true })
            setReload((n) => n + 1)
          }}
        />
      )}
    </section>
  )
}

// Uma versão como fonte: a linha do tempo do dataset (clicar troca a versão)
// e o painel dela (resumo e o que mudou).
function VersionSource({ dataset, versions, number, onPick }: { dataset: Dataset; versions: Version[]; number: number; onPick: (n: number) => void }) {
  const [profile, setProfile] = useState<AnnotationProfile | undefined>()
  const [current, setCurrent] = useState<Version | null>(null)
  const [error, setError] = useState('')
  const [tab, setTab] = useState<PanelTab>('summary')
  useEffect(() => {
    let active = true
    getAnnotationProfile(dataset.profile)
      .then((result) => { if (active) setProfile(result) })
      .catch(() => { /* sem o perfil: os cartões mostram as chaves */ })
    // o agora: montado na hora pelo backend (pode levar uns segundos)
    getCurrentVersion(dataset.name)
      .then((result) => { if (active) setCurrent(result) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível montar o estado atual.')
      })
    return () => { active = false }
  }, [dataset.profile, dataset.name])

  // 0 = o agora
  const version = number === 0 ? current : versions.find((item) => item.number === number) ?? current
  return (
    <>
      <DatasetTimeline
        actions={(
          <Link className="button button--secondary button--sm" to={`/versoes/${encodeURIComponent(dataset.name)}`}>Abrir o dataset</Link>
        )}
        current={current ?? undefined}
        dataset={dataset.name}
        onSelect={onPick}
        selected={version?.number}
        versions={versions}
      />
      {error && <p className="form-error">{error}</p>}
      {!version && !error && <p className="dashboard-hint">Montando o estado atual...</p>}
      {version && (
        <VersionPanel
          dataset={dataset.name}
          key={version.number}
          onTab={setTab}
          profile={profile}
          tab={tab}
          version={version}
          versions={versions}
        />
      )}
    </>
  )
}

// Apagar um dataset. Com versões, some junto o histórico (pede o nome
// digitado); as imagens e a anotação delas não mudam.
function DeleteDatasetModal({ name, versions, onClose, onDeleted }: { name: string; versions: number; onClose: () => void; onDeleted: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [typed, setTyped] = useState('')
  const needsName = versions > 0

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  async function remove() {
    setBusy(true)
    setError('')
    try {
      await deleteDataset(name, needsName)
      onDeleted()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar.')
      setBusy(false)
    }
  }

  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={() => { if (!busy) onClose() }} role="dialog">
      <div className="modal-dialog" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Apagar o dataset {name}?</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy} onClick={onClose} type="button">✕</button>
        </div>
        <div className="modal-body dashboard-delete">
          {needsName ? (
            <>
              <p className="modal-warning">
                Some junto o histórico: <strong>{versions} versão(ões)</strong>, com as imagens congeladas nelas, a linha do tempo e o “o que mudou”. Não dá para desfazer.
              </p>
              <p className="dashboard-hint">As imagens, as anotações e as caixas não mudam: só deixa de existir a foto que cada versão guardou.</p>
              <label htmlFor="delete-dataset-name">Digite <code>{name}</code> para confirmar</label>
              <input autoFocus id="delete-dataset-name" onChange={(event) => setTyped(event.target.value)} placeholder={name} value={typed} />
            </>
          ) : (
            <p className="dashboard-hint">Ele ainda não tem versões: some só a pesquisa salva (o perfil, os pacotes e os filtros). Nenhuma imagem é apagada.</p>
          )}
          {error && <p className="form-error">{error}</p>}
        </div>
        <div className="modal-actions">
          <button className="button button--secondary" disabled={busy} onClick={onClose} type="button">Cancelar</button>
          <button className="button button--danger" disabled={busy || (needsName && typed.trim() !== name)} onClick={() => void remove()} type="button">
            {busy ? 'Apagando...' : needsName ? 'Apagar com o histórico' : 'Apagar'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
