import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { useNavigate } from 'react-router-dom'
import { listPackageTasks, listPackages, type PackageSummary } from '../../services/api'
import type { Package } from '../../types/package'
import './Packages.css'

// "há 3 dias"; vazio quando a data falta (pacotes antigos vêm sem ela)
function formatUpdatedAt(date: string) {
  const time = new Date(date).getTime()
  if (!Number.isFinite(time) || new Date(date).getFullYear() < 2000) return ''
  const days = Math.max(0, Math.floor((Date.now() - time) / 86400000))
  if (days === 0) return 'hoje'
  return `há ${days} ${days === 1 ? 'dia' : 'dias'}`
}

function SearchIcon() {
  return (
    <svg aria-hidden="true" fill="none" height="16" viewBox="0 0 24 24" width="16">
      <circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="1.8" />
      <path d="m20 20-3.5-3.5" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" />
    </svg>
  )
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

type MenuOption = { value: string; label: string }

// Um menu "rótulo: valor ⌄" com as opções embaixo; fecha ao escolher ou ao
// clicar fora.
// marked: o filtro está aplicado (o rótulo fica em destaque).
function Menu({ label, value, options, onChange, marked = false }: { label: string; value: string; options: MenuOption[]; onChange: (value: string) => void; marked?: boolean }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const close = (event: MouseEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])
  const current = options.find((option) => option.value === value) ?? options[0]
  return (
    <div className="filter-menu" ref={ref}>
      <button aria-expanded={open} className={marked ? 'filter-button filter-button--set' : 'filter-button'} onClick={() => setOpen((isOpen) => !isOpen)} type="button">
        {label} <strong>{current?.label}</strong> <span>⌄</span>
      </button>
      {open && (
        <div className="filter-menu__options">
          {options.map((option) => (
            <button
              className={option.value === value ? 'filter-menu__option filter-menu__option--active' : 'filter-menu__option'}
              key={option.value}
              onClick={() => {
                onChange(option.value)
                setOpen(false)
              }}
              type="button"
            >
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// Quantas colunas a grade tem (1 = lista: a capa à esquerda, o nome à
// direita). Fica guardado no navegador de quem usa.
const COLUMNS = [5, 4, 3, 2, 1] as const
type Columns = (typeof COLUMNS)[number]
const COLUMNS_KEY = 'sidm.packages.columns'

function savedColumns(): Columns {
  try {
    const n = Number(localStorage.getItem(COLUMNS_KEY))
    return (COLUMNS as readonly number[]).includes(n) ? (n as Columns) : 3
  } catch {
    return 3
  }
}

function GridIcon({ columns }: { columns: Columns }) {
  if (columns === 1) {
    return (
      <svg aria-hidden="true" height="14" viewBox="0 0 16 14" width="16">
        <rect fill="currentColor" height="4" rx="1" width="5" x="0" y="0" />
        <rect fill="currentColor" height="1.6" rx="0.8" width="9" x="7" y="1.2" />
        <rect fill="currentColor" height="4" rx="1" width="5" x="0" y="5" />
        <rect fill="currentColor" height="1.6" rx="0.8" width="9" x="7" y="6.2" />
        <rect fill="currentColor" height="4" rx="1" width="5" x="0" y="10" />
        <rect fill="currentColor" height="1.6" rx="0.8" width="9" x="7" y="11.2" />
      </svg>
    )
  }
  const w = (16 - (columns - 1) * 1.5) / columns
  return (
    <svg aria-hidden="true" height="14" viewBox="0 0 16 14" width="16">
      {Array.from({ length: columns }, (_, i) => (
        <rect fill="currentColor" height="14" key={i} rx="1" width={w} x={i * (w + 1.5)} y="0" />
      ))}
    </svg>
  )
}

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
  const [columns, setColumns] = useState<Columns>(savedColumns)

  function chooseColumns(value: Columns) {
    setColumns(value)
    try {
      localStorage.setItem(COLUMNS_KEY, String(value))
    } catch {
      // sem armazenamento (modo privado): vale só nesta visita
    }
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
        <label className="search-field" htmlFor="package-search">
          <SearchIcon />
          <input
            id="package-search"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Pesquisar pacote pelo nome"
            type="search"
            value={query}
          />
        </label>
        <div className="package-toolbar__actions">
          {!loading && !error && (
            <span className="package-toolbar__count">{filteredPackages.length} de {packages.length} pacote(s)</span>
          )}
          <Menu label="Perfil" marked={Boolean(profileFilter)} onChange={setProfileFilter} options={profileOptions} value={profileFilter} />
          {taskOptions.length > 1 && <Menu label="Tarefa" marked={Boolean(taskFilter)} onChange={setTaskFilter} options={taskOptions} value={taskFilter} />}
          <Menu label="Ordenar por" onChange={(value) => setSortBy(value as SortBy)} options={SORTS} value={sortBy} />
          <div aria-label="Colunas da grade" className="grid-picker" role="group">
            {COLUMNS.map((value) => (
              <button
                aria-pressed={columns === value}
                className={columns === value ? 'grid-picker__option grid-picker__option--active' : 'grid-picker__option'}
                key={value}
                onClick={() => chooseColumns(value)}
                title={value === 1 ? 'Lista' : `${value} colunas`}
                type="button"
              >
                <GridIcon columns={value} />
              </button>
            ))}
          </div>
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
        <div className="empty-package">
          <p>Carregando pacotes...</p>
        </div>
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
