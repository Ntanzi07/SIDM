import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { useNavigate } from 'react-router-dom'
import { listPackageTasks, listPackages, type PackageSummary } from '../../services/api'
import type { Package } from '../../types/package'
import { GridPicker, Menu, SearchField, SearchIcon } from '../../components/Toolbar/Toolbar'
import { COLUMNS, savedColumns, saveColumns, type Columns, type MenuOption } from '../../components/Toolbar/columns'
import { PageLoader } from '../../components/PageLoader/PageLoader'
import './Packages.css'

// "há 3 dias"; vazio quando a data falta (pacotes antigos vêm sem ela)
function formatUpdatedAt(date: string) {
  const time = new Date(date).getTime()
  if (!Number.isFinite(time) || new Date(date).getFullYear() < 2000) return ''
  const days = Math.max(0, Math.floor((Date.now() - time) / 86400000))
  if (days === 0) return 'hoje'
  return `há ${days} ${days === 1 ? 'dia' : 'dias'}`
}

// index: a posição na grade, para a entrada em cascata (cada cartão sobe um
// pouco depois do anterior).
function PackageCard({ packageItem, index, onClick }: { packageItem: Package; index: number; onClick: () => void }) {
  const [coverFailed, setCoverFailed] = useState(false)
  const cover = packageItem.coverImage && !coverFailed ? packageItem.coverImage : ''
  const updated = formatUpdatedAt(packageItem.updatedAt)
  const meta = [packageItem.createdBy ? `por ${packageItem.createdBy}` : '', updated].filter(Boolean).join(' · ')
  const done = packageItem.imageCount > 0 ? Math.round((packageItem.verified / packageItem.imageCount) * 100) : 0
  return (
    <button className="package-card" onClick={onClick} style={{ '--i': Math.min(index, 12) } as CSSProperties} type="button">
      <div className={cover ? 'package-card__cover' : 'package-card__cover package-card__cover--empty'}>
        {cover ? <img alt="" loading="lazy" onError={() => setCoverFailed(true)} src={cover} /> : <span>▧</span>}
      </div>
      <div className="package-card__body">
        <span className="eyebrow">{packageItem.profile}</span>
        <h2>{packageItem.name}</h2>
        {meta && <p>{meta}</p>}
        {packageItem.tasks.length > 0 && (
          <div className="package-card__tasks" title={`Tarefas: ${packageItem.tasks.join(', ')}`}>
            {packageItem.tasks.slice(0, 3).map((task) => <span key={task}>{task}</span>)}
            {packageItem.tasks.length > 3 && <span>+{packageItem.tasks.length - 3}</span>}
          </div>
        )}
        <div className="package-card__progress" title={`${packageItem.verified} de ${packageItem.imageCount} verificadas`}>
          <span className="package-card__progress-track"><span style={{ width: `${done}%` }} /></span>
          <span className="package-card__progress-text">
            {packageItem.imageCount === 0 ? 'sem imagens' : `${packageItem.verified} de ${packageItem.imageCount} verificadas`}
          </span>
        </div>
      </div>
      <dl className="package-card__stats">
        <div>
          <dt>Imagens</dt>
          <dd>{packageItem.imageCount.toLocaleString('pt-BR')}</dd>
        </div>
        <div>
          <dt>Verificadas</dt>
          <dd>{packageItem.verified.toLocaleString('pt-BR')}</dd>
        </div>
        <div>
          <dt>Pendentes</dt>
          <dd>{packageItem.pending.toLocaleString('pt-BR')}</dd>
        </div>
        <div>
          <dt>Em anotação</dt>
          <dd>{packageItem.inProgress.toLocaleString('pt-BR')}</dd>
        </div>
      </dl>
    </button>
  )
}

const COLUMNS_KEY = 'sidm.packages.columns'

type SortBy = 'updated' | 'name' | 'creator' | 'profile' | 'task'

const SORTS: MenuOption[] = [
  { value: 'updated', label: 'última atualização' },
  { value: 'name', label: 'nome' },
  { value: 'creator', label: 'criador' },
  { value: 'profile', label: 'perfil de anotação' },
  { value: 'task', label: 'tarefa' },
]

export function PackagesPage() {
  const navigate = useNavigate()
  const [packages, setPackages] = useState<Package[]>([])
  const [query, setQuery] = useState('')
  const [sortBy, setSortBy] = useState<SortBy>('updated')
  const [profileFilter, setProfileFilter] = useState('') // vazio = todos
  const [taskFilter, setTaskFilter] = useState('')
  const [columns, setColumns] = useState<Columns>(() => savedColumns(COLUMNS_KEY, 3))

  function chooseColumns(value: Columns) {
    setColumns(value)
    saveColumns(COLUMNS_KEY, value)
  }
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    // as tarefas são um extra: sem elas a lista aparece do mesmo jeito
    Promise.all([listPackages(), listPackageTasks().catch(() => ({} as Record<string, string[]>))])
      .then(([items, tasks]: [PackageSummary[], Record<string, string[]>]) => {
        if (!active) return
        setPackages(items.map((item) => ({
          profile: item.profile,
          tasks: tasks[item.name] ?? [],
          name: item.name,
          createdBy: item.created_by,
          updatedAt: item.created_at,
          imageCount: item.total,
          verified: item.verified,
          pending: item.pending,
          inProgress: item.in_progress,
          coverImage: item.coverImage,
        })))
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os pacotes.')
      })
      .finally(() => {
        if (active) setLoading(false)
      })

    return () => {
      active = false
    }
  }, [])

  // as opções dos filtros saem dos próprios pacotes
  const profileOptions = useMemo(
    () => [{ value: '', label: 'todos' }, ...[...new Set(packages.map((item) => item.profile))].sort().map((name) => ({ value: name, label: name }))],
    [packages],
  )
  const taskOptions = useMemo(
    () => [{ value: '', label: 'todas' }, ...[...new Set(packages.flatMap((item) => item.tasks))].sort().map((name) => ({ value: name, label: name }))],
    [packages],
  )

  const filteredPackages = useMemo(
    () => packages
      .filter((item) => item.name.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
      .filter((item) => !profileFilter || item.profile === profileFilter)
      .filter((item) => !taskFilter || item.tasks.includes(taskFilter))
      .sort((first, second) => {
        if (sortBy === 'name') return first.name.localeCompare(second.name)
        if (sortBy === 'creator') return first.createdBy.localeCompare(second.createdBy)
        if (sortBy === 'profile') return first.profile.localeCompare(second.profile) || first.name.localeCompare(second.name)
        if (sortBy === 'task') {
          // pela primeira tarefa; os sem tarefa no fim
          const a = first.tasks[0] ?? '\uffff'
          const b = second.tasks[0] ?? '\uffff'
          return a.localeCompare(b) || first.name.localeCompare(second.name)
        }
        return new Date(second.updatedAt).getTime() - new Date(first.updatedAt).getTime()
      }),
    [packages, query, sortBy, profileFilter, taskFilter],
  )
  const filtering = Boolean(query || profileFilter || taskFilter)

  return (
    <section className="package-page">
      <div className="page-heading">
        <div>
          <h1>Pacotes</h1>
        </div>
        <div className="package-heading__actions">
          <button className="button button--secondary" onClick={() => navigate('/configurar')} type="button">
            Configurar
          </button>
          <button className="button button--primary" onClick={() => navigate('/pacotes/criar')} type="button">
            <span aria-hidden="true">+</span> Criar pacote
          </button>
        </div>
      </div>

      <div className="package-toolbar">
        <SearchField id="package-search" onChange={setQuery} placeholder="Pesquisar pacote pelo nome" value={query} />
        <div className="package-toolbar__actions">
          {!loading && !error && (
            <span className="package-toolbar__count">{filteredPackages.length} de {packages.length} pacote(s)</span>
          )}
          <Menu label="Perfil" marked={Boolean(profileFilter)} onChange={setProfileFilter} options={profileOptions} value={profileFilter} />
          {taskOptions.length > 1 && <Menu label="Tarefa" marked={Boolean(taskFilter)} onChange={setTaskFilter} options={taskOptions} value={taskFilter} />}
          <Menu label="Ordenar por" onChange={(value) => setSortBy(value as SortBy)} options={SORTS} value={sortBy} />
          <GridPicker onChange={chooseColumns} options={COLUMNS} value={columns} />
          {filtering && (
            <button className="filter-clear" onClick={() => { setQuery(''); setProfileFilter(''); setTaskFilter('') }} type="button">
              Limpar
            </button>
          )}
        </div>
      </div>

      {filteredPackages.length > 0 ? (
        <div
          className={columns === 1 ? 'package-grid package-grid--list package-grid--row' : columns === 2 ? 'package-grid package-grid--row' : 'package-grid'}
          data-cols={columns}
          style={{ '--cols': columns } as CSSProperties}
        >
          {filteredPackages.map((packageItem, index) => (
            <PackageCard
              index={index}
              key={packageItem.name}
              onClick={() => navigate(`/pacotes/${encodeURIComponent(packageItem.name)}`)}
              packageItem={packageItem}
            />
          ))}
        </div>
      ) : loading ? (
        <PageLoader label="Carregando os pacotes" />
      ) : error ? (
        <div className="empty-package">
          <h2>Não foi possível carregar os pacotes</h2>
          <p>{error}</p>
        </div>
      ) : (
        <div className="empty-package">
          <SearchIcon />
          <h2>Nenhum pacote encontrado</h2>
          <p>{profileFilter || taskFilter ? 'Nenhum pacote com esses filtros.' : 'Tente buscar por outro nome.'}</p>
        </div>
      )}
    </section>
  )
}
