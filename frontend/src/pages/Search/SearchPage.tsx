import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  createDataset,
  getImageStats,
  getVersionStats,
  getLabelStats,
  listAnnotationProfiles,
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
import { facetsOf, fmt, valuesOf } from '../../utils/stats'
import { CountCard, LabelCard } from '../../components/Stats/StatsCards'
import '../Packages/Packages.css'
import { useToastState } from '../../hooks/useToast'
import './Search.css'

// Pesquisa/dashboard: quantas imagens de UM perfil de anotação têm cada valor
// de cada campo. Clicar num valor filtra por ele.
// version: pesquisa dentro de uma versão congelada, ou do agora do dataset
// (só verificadas; sem lista de imagens nem salvar).
export type VersionSource = { dataset: string; number: number } // number 0 = o agora (não salvo)

export function SearchPage({ profileName, version }: { profileName: string; version?: VersionSource }) {
  const [profiles, setProfiles] = useState<AnnotationProfile[]>([])
  // só verificadas e todas as tarefas; o pacote filtra clicando na barra
  const status: StatsStatus = 'verified'
  const [chosenPackages, setChosenPackages] = useState<string[]>([]) // vazio = todos
  const [labels, setLabels] = useState<LabelStats | null>(null)
  const [showImages, setShowImages] = useState(false) // a lista de imagens fica aberta entre as pesquisas
  const [filters, setFilters] = useState<Record<string, string>>({})
  const [stats, setStats] = useState<ImageStats | null>(null)
  const [, setError] = useToastState('error')
  const [saving, setSaving] = useState(false) // o formulário de "Salvar como dataset" aberto
  // a pesquisa da tela (perfil + filtros) e a da última resposta: diferentes = carregando
  const requestKey = JSON.stringify([profileName, status, chosenPackages, filters])
  const [resultKey, setResultKey] = useState('')
  const loading = resultKey !== requestKey

  const profile = profiles.find((item) => item.name === profileName)
  const facets = useMemo(() => (profile ? facetsOf(profile) : []), [profile])

  useEffect(() => {
    let active = true
    listAnnotationProfiles()
      .then((items) => { if (active) setProfiles(items) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis.')
      })
    return () => { active = false }
  }, [setError])

  // numa versão: tudo numa chamada só (contas e caixas dos itens congelados)
  const versionKey = version ? `${version.dataset}#${version.number}` : ''
  useEffect(() => {
    if (!version) return
    let active = true
    const key = JSON.stringify([profileName, status, chosenPackages, filters])
    getVersionStats(version.dataset, version.number, { packages: chosenPackages, filters }, [])
      .then((result) => {
        if (!active) return
        setStats(result.images)
        setLabels(result.labels)
        setError('')
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível pesquisar na versão.')
      })
      .finally(() => { if (active) setResultKey(key) })
    return () => { active = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- versionKey resume a versão
  }, [versionKey, profileName, chosenPackages, filters])

  useEffect(() => {
    if (!profileName || version) return
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
  }, [profileName, chosenPackages, filters, version, setError])

  // As labels: as mesmas imagens da pesquisa, nas tarefas escolhidas.
  useEffect(() => {
    if (!profileName || version) return
    let active = true
    getLabelStats({ profile: profileName, status, packages: chosenPackages, filters }, [])
      .then((result) => { if (active) setLabels(result) })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível contar as labels.')
      })
    return () => { active = false }
  }, [profileName, chosenPackages, filters, version, setError])

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
    <div className="search-body">
      {profile && saving && (
        <SaveDataset
          onCancel={() => setSaving(false)}
          profile={profileName}
          query={{
            packages: chosenPackages,
            filters: Object.entries(filters).map(([key, value]) => ({ key, value })),
            tasks: [],
          }}
          status={status}
        />
      )}

      {/* Sem cartão de busca: os filtros vêm de clicar nos gráficos (a rosca,
          as barras), e aparecem aqui como etiquetas para tirar. */}
      <div className="search-filterbar">
        <span className="eyebrow">Filtros</span>
        {chosenPackages.map((name) => (
          <button className="search-filterbar__chip" key={`pkg-${name}`} onClick={() => setChosenPackages([])} title="Tirar o filtro" type="button">
            Pacote: <strong>{name}</strong> <span aria-hidden="true">✕</span>
          </button>
        ))}
        {activeFilters === 0 && chosenPackages.length === 0 && <span className="search-filterbar__hint">nenhum · clique num valor dos gráficos para filtrar</span>}
        {Object.entries(filters).map(([key, value]) => {
          const facet = facets.find((item) => item.key === key)
          const label = facet ? valuesOf(facet.field).find((item) => item.value === value)?.label ?? value : value
          return (
            <button className="search-filterbar__chip" key={key} onClick={() => setFilter(key, '')} title="Tirar o filtro" type="button">
              {facet?.title ?? key}: <strong>{label}</strong> <span aria-hidden="true">✕</span>
            </button>
          )
        })}
        {activeFilters + chosenPackages.length > 1 && (
          <button className="search-filterbar__clear" onClick={() => { setFilters({}); setChosenPackages([]) }} type="button">Limpar</button>
        )}
        {!version && profile && !saving && (
          <button className="button button--secondary button--sm search-filterbar__save" onClick={() => setSaving(true)} type="button">
            Salvar como dataset
          </button>
        )}
      </div>

      {!profile && profiles.length > 0 && <p className="profile-name-hint">O perfil {profileName} não existe.</p>}

      {profile && stats && (
        <div className={loading ? 'search-results search-results--loading' : 'search-results'}>

          {labels && labels.tasks.length > 0 && (
            <>
              <h2 className="search-section">Caixas por label</h2>
              <div className="search-grid">
                {labels.tasks.map((task) => <LabelCard key={task.task} searchImages={labels.images} task={task} />)}
              </div>
              <h2 className="search-section">Campos da anotação</h2>
            </>
          )}
          <div className="search-grid">
            <CountCard
              headline={{
                value: stats.total,
                caption: `imagem(ns) verificada(s)${activeFilters + chosenPackages.length > 0 ? ' com os filtros' : ''}${version ? (version.number === 0 ? ' agora' : ` na v${version.number}`) : ''}`,
              }}
              active={chosenPackages[0]}
              onPick={(name) => setChosenPackages((current) => (current[0] === name ? [] : [name]))}
              rows={Object.keys(stats.packages).sort().map((name) => ({ value: name, label: name, count: stats.packages[name] }))}
              of={stats.total}
              title="Imagens por pacote"
            />
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
                  donut={facet.field.type !== 'multi'}
                  key={facet.key}
                  missing={missing}
                  note={facet.field.type === 'multi' ? 'Uma imagem pode ter vários valores: a soma passa do total.' : facet.objectType ? 'Conta objetos.' : undefined}
                  of={of}
                  onPick={(value) => setFilter(facet.key, filters[facet.key] === value ? '' : value)}
                  rows={rows}
                  title={facet.title}
                  unit={facet.objectType ? 'objetos' : 'imagens'}
                />
              )
            })}
          </div>


          {!version && <ImageList
            key={requestKey}
            onOpen={() => setShowImages(true)}
            open={showImages}
            search={{ profile: profileName, status, packages: chosenPackages, filters }}
            total={stats.total}
          />}
        </div>
      )}
    </div>
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
  const [error, setError] = useToastState('error')
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
  }, [open, searchKey, setError])

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
  const [, setError] = useToastState('error')

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const dataset = await createDataset({ name: name.trim(), profile, description, query })
      navigate(`/dashboard/${encodeURIComponent(dataset.profile)}?dataset=${encodeURIComponent(dataset.name)}`, { state: { fresh: Date.now() } })
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
