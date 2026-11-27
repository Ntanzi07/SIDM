import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { deleteDataset, getAnnotationProfile, getCurrentVersion, listAnnotationProfiles, listDatasets, listPackageSummaries, listVersions, type Dataset, type Version } from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import { SearchPage } from '../Search/SearchPage'
import { DatasetTimeline, NewVersionModal, VersionPanel, type PanelTab } from '../Versions/VersionParts'
import { formatWhen } from '../Versions/format'
import { Sparkline } from '../../components/Charts/Sparkline'
import { fmt } from '../../utils/stats'
import { useToastState } from '../../hooks/useToast'
import { useIsAdmin } from '../../services/user'
import { Bone, Skeleton } from '../../components/Skeleton/Skeleton'
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

// Os números de cada perfil no seletor: as imagens (verificadas de quantas),
// os pacotes e os datasets. Tudo de dois pedidos (pacotes e datasets).
type ProfileNumbers = { packages: number; total: number; verified: number; datasets: number }

function ProfilePicker() {
  const [profiles, setProfiles] = useState<AnnotationProfile[] | null>(null)
  const [numbers, setNumbers] = useState<Record<string, ProfileNumbers> | null>(null)
  const [error, setError] = useToastState('error')

  useEffect(() => {
    let active = true
    listAnnotationProfiles()
      .then((items) => { if (active) setProfiles(items) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis.')
      })
    // os números são um extra: sem eles os cartões aparecem com o nome
    Promise.all([listPackageSummaries(), listDatasets().catch(() => [] as Dataset[])])
      .then(([packages, datasets]) => {
        if (!active) return
        const out: Record<string, ProfileNumbers> = {}
        const of = (name: string) => (out[name] ??= { packages: 0, total: 0, verified: 0, datasets: 0 })
        for (const p of packages) {
          const n = of(p.profile)
          n.packages += 1
          n.total += p.total
          n.verified += p.verified
        }
        for (const d of datasets) of(d.profile).datasets += 1
        setNumbers(out)
      })
      .catch(() => { if (active) setNumbers({}) })
    return () => { active = false }
  }, [setError])

  // os com imagens primeiro (os vazios no fim, apagados)
  const sorted = profiles && [...profiles].sort((a, b) => (numbers?.[b.name]?.total ?? 0) - (numbers?.[a.name]?.total ?? 0))

  return (
    <section className="dashboard-page">
      <div className="page-heading">
        <div>
          <h1>Dashboard</h1>
          <p>Escolha um perfil de anotação para ver os números das imagens, as caixas por label e as versões dos datasets dele.</p>
        </div>
      </div>
      {profiles === null && !error && (
        <Skeleton label="Carregando os perfis">
          <div className="profile-grid">
            {[220, 170, 250].map((w, i) => (
              <div className="profile-card sk-card" key={i}>
                <Bone h={26} w={w * 0.7} />
                <Bone h={12} w="85%" />
                <Bone h={34} r={8} w={140} />
                <Bone h={6} r={999} />
                <Bone h={12} w={150} />
              </div>
            ))}
          </div>
        </Skeleton>
      )}
      {sorted && (
        <div className="profile-grid">
          {sorted.map((item, index) => {
            // números carregados: quem não aparece neles não tem pacote nem dataset
            const n = numbers ? numbers[item.name] ?? { packages: 0, total: 0, verified: 0, datasets: 0 } : undefined
            const percent = n && n.total > 0 ? Math.round((n.verified / n.total) * 100) : 0
            return (
              <Link
                className={n && n.total === 0 ? 'profile-card profile-card--empty' : 'profile-card'}
                key={item.name}
                style={{ animationDelay: `${Math.min(index, 10) * 45}ms` }}
                to={`/dashboard/${encodeURIComponent(item.name)}`}
              >
                <h2>{item.name}</h2>
                {item.description && <p className="profile-card__desc">{item.description}</p>}
                {!n ? (
                  <Bone h={34} r={8} w={140} />
                ) : n.total === 0 ? (
                  <p className="profile-card__none">Nenhuma imagem ainda</p>
                ) : (
                  <>
                    <p className="profile-card__big">
                      <strong>{fmt(n.verified)}</strong>
                      <span>de {fmt(n.total)} {n.total === 1 ? 'imagem verificada' : 'imagens verificadas'}</span>
                    </p>
                    <span aria-label={`${percent}% verificadas`} className="profile-card__bar" role="img">
                      <span style={{ width: `${percent}%` }} />
                    </span>
                  </>
                )}
                <p className="profile-card__meta">
                  {n ? (
                    <>
                      {n.packages} {n.packages === 1 ? 'pacote' : 'pacotes'}, {n.datasets} {n.datasets === 1 ? 'dataset' : 'datasets'}
                      {n.total > 0 && <>, {percent}% verificado</>}
                    </>
                  ) : '\u00a0'}
                </p>
              </Link>
            )
          })}
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
  const [error, setError] = useToastState('error')
  const [reload, setReload] = useState(0) // muda depois de criar uma versão
  const [creating, setCreating] = useState('') // o dataset do pop-up "Nova versão"
  const [deleting, setDeleting] = useState('') // o dataset do pop-up "Apagar"
  const [description, setDescription] = useState('')
  useEffect(() => {
    let active = true
    getAnnotationProfile(profile)
      .then((item) => { if (active) setDescription(item.description ?? '') })
      .catch(() => { /* sem a descrição: fica o texto padrão */ })
    return () => { active = false }
  }, [profile])

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
  }, [profile, reload, setError])

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
          <h1>{profile}</h1>
          <p>{description || 'Os números das imagens verificadas deste perfil, as caixas por label e as versões dos datasets.'}</p>
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

          <span className="dashboard-side__group">Versões</span>
          {datasets === null && (
            <Skeleton label="Carregando as versões">
              {[0, 1].map((i) => (
                <div className="source-item sk-source" key={i}><Bone h={14} w={[120, 90][i]} /><Bone h={11} w={150} /></div>
              ))}
            </Skeleton>
          )}
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
  const [error, setError] = useToastState('error')
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
  }, [dataset.profile, dataset.name, setError])

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
      {!version && !error && (
        <Skeleton label="Montando o estado atual">
          <div className="sk-title sk-panel-head"><Bone h={10} w={160} /><Bone h={28} r={8} w={220} /><Bone h={13} w="60%" /></div>
        </Skeleton>
      )}
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
  const [, setError] = useToastState('error')
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
