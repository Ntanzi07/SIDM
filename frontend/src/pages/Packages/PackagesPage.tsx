import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { listPackages, type PackageSummary } from '../../services/api'
import type { Package } from '../../types/package'
import './Packages.css'

function formatUpdatedAt(date: string) {
  const days = Math.max(0, Math.floor((Date.now() - new Date(date).getTime()) / 86400000))
  return `há ${days} ${days === 1 ? 'dia' : 'dias'}`
}

function PackageCard({ packageItem, onClick }: { packageItem: Package; onClick: () => void }) {
  return (
    <button className="package-card" onClick={onClick} type="button">
      <div className={`package-card__cover ${!packageItem.coverImage ? 'package-card__cover--empty' : ''}`}>
        {packageItem.coverImage ? <img alt="" src={packageItem.coverImage} /> : <span>▧</span>}
      </div>
      <div className="package-card__body">
        <h2>{packageItem.name}</h2>
        <p>Criado por {packageItem.createdBy}</p>
        <span className="package-card__updated">Última atualização {formatUpdatedAt(packageItem.updatedAt)}</span>
      </div>
    </button>
  )
}

export function PackagesPage() {
  const navigate = useNavigate()
  const [packages, setPackages] = useState<Package[]>([])
  const [query, setQuery] = useState('')
  const [sortBy, setSortBy] = useState<'name' | 'creator' | 'updated'>('updated')
  const [filterOpen, setFilterOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    listPackages()
      .then((items: PackageSummary[]) => {
        if (!active) return
        setPackages(items.map((item) => ({
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

  const filteredPackages = useMemo(
    () => packages
      .filter((item) => item.name.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
      .sort((first, second) => {
        if (sortBy === 'name') return first.name.localeCompare(second.name)
        if (sortBy === 'creator') return first.createdBy.localeCompare(second.createdBy)
        return new Date(second.updatedAt).getTime() - new Date(first.updatedAt).getTime()
      }),
    [packages, query, sortBy],
  )

  return (
    <section className="package-page">
      <div className="package-toolbar">
        <label className="search-field" htmlFor="package-search">
          <span>⌕</span>
          <input
            id="package-search"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Pesquisar pacote pelo nome"
            value={query}
          />
        </label>
        <div className="package-toolbar__actions">
          <div className="filter-menu">
            <button className="filter-button" onClick={() => setFilterOpen((isOpen) => !isOpen)} type="button">
              Filtrar por <strong>{sortBy === 'updated' ? 'última atualização' : sortBy === 'name' ? 'nome' : 'criador'}</strong> <span>⌄</span>
            </button>
            {filterOpen && (
              <div className="filter-menu__options">
                {([
                  ['updated', 'Última atualização'],
                  ['name', 'Nome'],
                  ['creator', 'Criador'],
                ] as const).map(([value, label]) => (
                  <button
                    className={sortBy === value ? 'filter-menu__option filter-menu__option--active' : 'filter-menu__option'}
                    key={value}
                    onClick={() => {
                      setSortBy(value)
                      setFilterOpen(false)
                    }}
                    type="button"
                  >
                    {label}
                  </button>
                ))}
              </div>
            )}
          </div>
          <button className="button button--secondary package-toolbar__settings" onClick={() => navigate('/configurar')} type="button">
            Configurar
          </button>
          <button className="button button--primary package-toolbar__create" onClick={() => navigate('/pacotes/criar')} type="button">
            <span>+</span> Criar pacote
          </button>
        </div>
      </div>

      {filteredPackages.length > 0 ? (
        <div className="package-grid">
          {filteredPackages.map((packageItem) => (
            <PackageCard key={packageItem.name} onClick={() => navigate(`/pacotes/${encodeURIComponent(packageItem.name)}`)} packageItem={packageItem} />
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
          <span>⌕</span>
          <h2>Nenhum pacote encontrado</h2>
          <p>Tente buscar por outro nome.</p>
        </div>
      )}
    </section>
  )
}
