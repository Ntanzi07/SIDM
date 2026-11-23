import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  createDataset,
  getImageStats,
  getLabelStats,
  listAnnotationProfiles,
  listPackagesOfProfile,
  listTasks,
  searchImages,
  type Counts,
  type DatasetQuery,
  type SearchImage,
  type StatsQuery,
  type ImageStats,
  type LabelStats,
  type StatsStatus,
} from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import { fieldLabel } from '../../utils/annotationForm'
import { facetsOf, fmt, toggle, valuesOf } from '../../utils/stats'
import { Chips, CountCard, LabelCard } from '../../components/Stats/StatsCards'
import '../Packages/Packages.css'
import './Search.css'

// Pesquisa/dashboard: quantas imagens de UM perfil de anotação têm cada valor
// de cada campo. Clicar num valor filtra por ele.
export function SearchPage() {
  const [profiles, setProfiles] = useState<AnnotationProfile[]>([])
  const [profileName, setProfileName] = useState('')
  const [status, setStatus] = useState<StatsStatus>('verified')
  const [chosenPackages, setChosenPackages] = useState<string[]>([]) // vazio = todos
  const [packages, setPackages] = useState<string[]>([])
  const [tasks, setTasks] = useState<string[]>([])
  const [allTasks, setAllTasks] = useState(true)
  const [chosenTasks, setChosenTasks] = useState<string[]>([])
  const [labels, setLabels] = useState<LabelStats | null>(null)
  const [showImages, setShowImages] = useState(false) // a lista de imagens fica aberta entre as pesquisas
  const [filters, setFilters] = useState<Record<string, string>>({})
  const [stats, setStats] = useState<ImageStats | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false) // o formulário de "Salvar como dataset" aberto
  // a pesquisa da tela (perfil + filtros) e a da última resposta: diferentes = carregando
  const requestKey = JSON.stringify([profileName, status, chosenPackages, filters])
  const [resultKey, setResultKey] = useState('')
  const loading = resultKey !== requestKey

  const profile = profiles.find((item) => item.name === profileName)
  const facets = useMemo(() => (profile ? facetsOf(profile) : []), [profile])

  useEffect(() => {
    let active = true
    listTasks()
      .then((items) => { if (active) setTasks(items.map((item) => item.name).sort()) })
      .catch(() => { if (active) setTasks([]) })
    listAnnotationProfiles()
      .then((items) => { if (active) setProfiles(items) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis.')
      })
    return () => { active = false }
  }, [])

  // Trocar de perfil começa uma pesquisa nova.
  function chooseProfile(name: string) {
    setProfileName(name)
    setChosenPackages([])
    setFilters({})
    setStats(null)
    setLabels(null)
    setPackages([])
    if (name) {
      listPackagesOfProfile(name).then(setPackages).catch(() => setPackages([]))
    }
  }

  useEffect(() => {
    if (!profileName) return
    let active = true
    const key = JSON.stringify([profileName, status, chosenPackages, filters])
    getImageStats({ profile: profileName, status, packages: chosenPackages, filters })
      .then((result) => {
        if (!active) return
        setStats(result)
        setError('')
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível pesquisar.')
      })
      .finally(() => { if (active) setResultKey(key) })
    return () => { active = false }
  }, [profileName, status, chosenPackages, filters])

  // As labels: as mesmas imagens da pesquisa, nas tarefas escolhidas.
  const noTasks = !allTasks && chosenTasks.length === 0
  useEffect(() => {
    if (!profileName || noTasks) return
    let active = true
    getLabelStats({ profile: profileName, status, packages: chosenPackages, filters }, allTasks ? [] : chosenTasks)
      .then((result) => { if (active) setLabels(result) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível contar as labels.')
      })
    return () => { active = false }
  }, [profileName, status, chosenPackages, filters, allTasks, chosenTasks, noTasks])

  function setFilter(key: string, value: string) {
    setFilters((current) => {
      const next = { ...current }
      if (value) next[key] = value
      else delete next[key]
      return next
    })
  }

  const activeFilters = Object.keys(filters).length

  return (
    <section className="search-page">
      <div className="page-heading">
        <div>
          <h1>Pesquisa</h1>
          <p>Quantas imagens de um perfil de anotação têm cada valor, por pacote. Clique num valor para filtrar por ele.</p>
        </div>
        {profile && !saving && (
          <button className="button button--secondary" onClick={() => setSaving(true)} type="button">
            Salvar como dataset
          </button>
        )}
      </div>
      {error && <p className="form-error">{error}</p>}
      {profile && saving && (
        <SaveDataset
          onCancel={() => setSaving(false)}
          profile={profileName}
          query={{
            packages: chosenPackages,
            filters: Object.entries(filters).map(([key, value]) => ({ key, value })),
            tasks: allTasks ? [] : chosenTasks,
          }}
          status={status}
        />
      )}

      <div className="create-card search-controls">
        <div className="search-controls__row">
          <label className="search-select">
            <span>Perfil de anotação</span>
            <select onChange={(event) => chooseProfile(event.target.value)} value={profileName}>
              <option value="">Escolha um perfil</option>
              {profiles.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
            </select>
          </label>
          {profile && (
            <>
              <label className="search-select">
                <span>Imagens</span>
                <select onChange={(event) => setStatus(event.target.value as StatsStatus)} value={status}>
                  <option value="verified">Verificadas</option>
                  <option value="pending">Pendentes</option>
                  <option value="all">Todas</option>
                </select>
              </label>
            </>
          )}
        </div>

        {profile && (
          <Chips
            all={chosenPackages.length === 0}
            allLabel="Todos"
            chosen={chosenPackages}
            empty="Nenhum pacote usa este perfil."
            items={packages}
            onAll={() => setChosenPackages([])}
            onToggle={(name) => setChosenPackages((current) => toggle(current, name))}
            title="Pacotes"
          />
        )}
        {profile && (
          <Chips
            all={allTasks}
            allLabel="Todas"
            chosen={chosenTasks}
            empty="Nenhuma tarefa cadastrada."
            items={tasks}
            onAll={() => { setAllTasks(true); setChosenTasks([]) }}
            onToggle={(name) => { setAllTasks(false); setChosenTasks((current) => toggle(allTasks ? [] : current, name)) }}
            title="Tarefas (labels do CVAT)"
          />
        )}

        {profile && facets.length > 0 && (
          <div className="search-controls__filters">
            {facets.map((facet) => (
              <label className="search-select" key={facet.key}>
                <span>{facet.title}</span>
                <select onChange={(event) => setFilter(facet.key, event.target.value)} value={filters[facet.key] ?? ''}>
                  <option value="">Todos</option>
                  {valuesOf(facet.field).map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
                </select>
              </label>
            ))}
          </div>
        )}
        {activeFilters > 0 && (
          <button className="button button--secondary button--sm" onClick={() => setFilters({})} type="button">
            Limpar filtros ({activeFilters})
          </button>
        )}
      </div>

      {!profile && <p className="profile-name-hint">Escolha um perfil de anotação para começar.</p>}

      {profile && stats && (
        <div className={loading ? 'search-results search-results--loading' : 'search-results'}>
          <div className="search-total">
            <strong>{fmt(stats.total)}</strong>
            <span>
              imagem(ns) {status === 'verified' ? 'verificada(s)' : status === 'pending' ? 'pendente(s)' : ''} no perfil {stats.profile}
              {activeFilters > 0 ? ' com os filtros' : ''}
            </span>
          </div>

          {!noTasks && labels && labels.tasks.length > 0 && (
            <>
              <h2 className="search-section">Labels do CVAT</h2>
              <div className="search-grid">
                {labels.tasks.map((task) => <LabelCard key={task.task} searchImages={labels.images} task={task} />)}
              </div>
              <h2 className="search-section">Campos da anotação</h2>
            </>
          )}
          <div className="search-grid">
            <CountCard
              rows={Object.keys(stats.packages).sort().map((name) => ({ value: name, label: name, count: stats.packages[name] }))}
              of={stats.total}
              title="Por pacote"
            />
            {status === 'all' && (
              <CountCard
                rows={[
                  { value: 'verified', label: 'Verificadas', count: stats.status.verified ?? 0 },
                  { value: 'pending', label: 'Pendentes', count: stats.status.pending ?? 0 },
                ]}
                of={stats.total}
                title="Por status"
              />
            )}
            {Object.entries(stats.objects).map(([type, item]) => (
              <div className="search-card" key={type}>
                <h2>{fieldLabel(profile.objects.find((object) => object.key === type) ?? { key: type })}</h2>
                <p className="search-card__big">{fmt(item.count)} <span>objeto(s) em {fmt(item.images)} imagem(ns)</span></p>
              </div>
            ))}
            {facets.map((facet) => {
              const counts: Counts | undefined = facet.objectType
                ? stats.objects[facet.objectType]?.fields[facet.field.key]
                : stats.fields[facet.field.key]
              const missing = facet.objectType
                ? stats.objects[facet.objectType]?.missing[facet.field.key] ?? 0
                : stats.missing[facet.field.key] ?? 0
              const of = facet.objectType ? stats.objects[facet.objectType]?.count ?? 0 : stats.total
              const known = valuesOf(facet.field)
              // valores guardados que o perfil não tem mais também aparecem
              const extra = Object.keys(counts ?? {}).filter((value) => !known.some((item) => item.value === value))
              const rows = [...known, ...extra.map((value) => ({ value, label: value }))]
                .map((item) => ({ ...item, count: counts?.[item.value] ?? 0 }))
              return (
                <CountCard
                  active={filters[facet.key]}
                  key={facet.key}
                  missing={missing}
                  note={facet.field.type === 'multi' ? 'Uma imagem pode ter vários valores: a soma passa do total.' : facet.objectType ? 'Conta objetos.' : undefined}
                  of={of}
                  onPick={(value) => setFilter(facet.key, filters[facet.key] === value ? '' : value)}
                  rows={rows}
                  title={facet.title}
                />
              )
            })}
          </div>


          <ImageList
            key={requestKey}
            onOpen={() => setShowImages(true)}
            open={showImages}
            search={{ profile: profileName, status, packages: chosenPackages, filters }}
            total={stats.total}
          />
        </div>
      )}
    </section>
  )
}

const IMAGES_PAGE = 50

// As imagens da pesquisa, como na tela do pacote: o nome abre a anotação
// (verificada: o visualizador). Fechada até pedir, para não buscar à toa;
// depois de aberta, fica aberta. A key muda com a pesquisa: a lista recomeça.
function ImageList({ search, total, open, onOpen }: { search: StatsQuery; total: number; open: boolean; onOpen: () => void }) {
  const navigate = useNavigate()
  const [items, setItems] = useState<SearchImage[]>([])
  const [count, setCount] = useState(0)
  const [loaded, setLoaded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const searchKey = JSON.stringify(search)

  // a primeira página, quando a lista está aberta
  useEffect(() => {
    if (!open) return
    let active = true
    searchImages(JSON.parse(searchKey) as StatsQuery, IMAGES_PAGE, 0)
      .then((page) => {
        if (!active) return
        setItems(page.items)
        setCount(page.total)
        setError('')
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível buscar as imagens.')
      })
      .finally(() => { if (active) setLoaded(true) })
    return () => { active = false }
  }, [open, searchKey])

  async function loadMore() {
    setBusy(true)
    try {
      const page = await searchImages(search, IMAGES_PAGE, items.length)
      setItems((current) => [...current, ...page.items])
      setCount(page.total)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível buscar as imagens.')
    } finally {
      setBusy(false)
    }
  }

  function openImage(image: SearchImage) {
    const pkg = encodeURIComponent(image.package)
    navigate(image.status === 'verified' ? `/pacotes/${pkg}/verificadas?sha1=${image.sha1}` : `/pacotes/${pkg}/anotacao?imagem=${image.sha1}`)
  }

  return (
    <div className="search-images">
      <h2 className="search-section">Imagens</h2>
      {!open ? (
        <button className="button button--secondary" disabled={total === 0} onClick={onOpen} type="button">
          Ver as {fmt(total)} imagem(ns) desta pesquisa
        </button>
      ) : (
        <div className="create-card search-images__card">
          {error && <p className="form-error">{error}</p>}
          {!loaded && <p className="profile-name-hint">Carregando...</p>}
          {loaded && items.length === 0 && !error && <p className="profile-name-hint">Nenhuma imagem nesta pesquisa.</p>}
          {items.length > 0 && (
            <div className="image-list">
              {items.map((image) => (
                <div className="image-list__item" key={image.sha1}>
                  <button
                    className="image-list__name image-list__open"
                    onClick={() => openImage(image)}
                    title={image.status === 'verified' ? 'Ver a anotação' : 'Abrir na anotação'}
                    type="button"
                  >
                    {image.original_name}
                  </button>
                  <div className="image-list__actions">
                    <span className="search-images__package">{image.package}</span>
                    <span className={image.status === 'verified' ? 'image-status image-status--verified' : 'image-status'}>
                      <i className={image.status === 'verified' ? 'status-dot status-dot--verified' : 'status-dot status-dot--pending'} />
                      {image.status === 'verified' ? 'Analisada' : 'Pendente'}
                    </span>
                  </div>
                </div>
              ))}
              {items.length < count && (
                <button className="button button--secondary image-list__more" disabled={busy} onClick={() => void loadMore()} type="button">
                  {busy ? 'Carregando...' : `Carregar mais (${fmt(items.length)} de ${fmt(count)})`}
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// Salva a pesquisa da tela (perfil, pacotes, filtros, tarefas) como um
// dataset, para congelar versões dele na tela de Versões.
function SaveDataset({ profile, query, status, onCancel }: {
  profile: string
  query: DatasetQuery
  status: StatsStatus
  onCancel: () => void
}) {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const dataset = await createDataset({ name: name.trim(), profile, description, query })
      navigate(`/versoes/${encodeURIComponent(dataset.name)}`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar o dataset.')
      setBusy(false)
    }
  }

  return (
    <form className="create-card save-dataset" onSubmit={(event) => void submit(event)}>
      <h2>Salvar como dataset</h2>
      <p className="profile-name-hint">
        Guarda o perfil {profile}, os pacotes, os filtros e as tarefas escolhidos. Cada versão do dataset congela as imagens que passam nesta pesquisa naquele momento.
        {status !== 'verified' && ' Só as verificadas entram numa versão (o filtro de status não é guardado).'}
      </p>
      {error && <p className="form-error">{error}</p>}
      <div className="form-row">
        <div className="form-field">
          <label htmlFor="dataset-name">Nome</label>
          <input autoFocus id="dataset-name" onChange={(event) => setName(event.target.value)} placeholder="ex.: motos-noite" value={name} />
        </div>
        <div className="form-field">
          <label htmlFor="dataset-description">Descrição (opcional)</label>
          <input id="dataset-description" onChange={(event) => setDescription(event.target.value)} placeholder="Para que serve" value={description} />
        </div>
      </div>
      <div className="save-dataset__actions">
        <button className="button button--primary" disabled={busy || !name.trim()} type="submit">{busy ? 'Salvando...' : 'Salvar'}</button>
        <button className="button button--secondary" disabled={busy} onClick={onCancel} type="button">Cancelar</button>
      </div>
    </form>
  )
}
