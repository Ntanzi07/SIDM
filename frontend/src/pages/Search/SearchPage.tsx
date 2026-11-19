import { useEffect, useMemo, useState } from 'react'
import {
  getImageStats,
  getLabelStats,
  listAnnotationProfiles,
  listPackagesOfProfile,
  listTasks,
  type Counts,
  type ImageStats,
  type LabelStats,
  type StatsStatus,
} from '../../services/api'
import type { AnnotationProfile, ProfileField } from '../../types/annotation'
import { fieldLabel, optionLabel } from '../../utils/annotationForm'
import '../Packages/Packages.css'
import './Search.css'

const fmt = (n: number) => n.toLocaleString('pt-BR')
const pct = (n: number, of: number) => (of > 0 ? `${Math.round((n / of) * 100)}%` : '0%')

// A pesquisa conta por valor só os campos de lista e sim/não (como o backend).
const countable = (field: ProfileField) => field.type === 'choice' || field.type === 'multi' || field.type === 'bool'

// Um campo que a tela mostra: da imagem ("fields.<campo>") ou de um tipo de
// objeto sem caixa ("objects.<tipo>.<campo>").
type Facet = {
  key: string // a chave do filtro na query
  title: string
  field: ProfileField
  objectType?: string
}

function facetsOf(profile: AnnotationProfile): Facet[] {
  const out: Facet[] = []
  for (const field of profile.fields.filter(countable)) {
    out.push({ key: `fields.${field.key}`, title: fieldLabel(field), field })
  }
  for (const type of profile.objects.filter((item) => !item.box)) {
    for (const field of type.fields.filter(countable)) {
      out.push({ key: `objects.${type.key}.${field.key}`, title: `${fieldLabel(type)}: ${fieldLabel(field)}`, field, objectType: type.key })
    }
  }
  return out
}

// Os valores possíveis, na ordem do perfil (os que não aparecem contam zero).
function valuesOf(field: ProfileField): Array<{ value: string; label: string }> {
  if (field.type === 'bool') return [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }]
  return (field.options ?? []).map((option) => ({ value: option.value, label: optionLabel(field, option.value) }))
}

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
  const [filters, setFilters] = useState<Record<string, string>>({})
  const [stats, setStats] = useState<ImageStats | null>(null)
  const [error, setError] = useState('')
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
      </div>
      {error && <p className="form-error">{error}</p>}

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
                {labels.tasks.map((task) => (
                  <CountCard
                    key={task.task}
                    note={`${fmt(task.images)} de ${fmt(labels.images)} imagem(ns) anotada(s) nesta tarefa · ${fmt(task.boxes)} caixa(s). A % é das caixas.`}
                    of={task.boxes}
                    rows={Object.keys(task.labels).sort().map((label) => ({
                      value: label,
                      label,
                      count: task.labels[label].boxes,
                      detail: `em ${fmt(task.labels[label].images)} img`,
                    }))}
                    title={`Tarefa ${task.task}`}
                  />
                ))}
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

        </div>
      )}
    </section>
  )
}

const toggle = (list: string[], item: string) => (list.includes(item) ? list.filter((x) => x !== item) : [...list, item])

// Uma escolha de vários: "Todos" ou os itens marcados.
function Chips({ title, items, chosen, all, allLabel, empty, onAll, onToggle }: {
  title: string
  items: string[]
  chosen: string[]
  all: boolean
  allLabel: string
  empty: string
  onAll: () => void
  onToggle: (item: string) => void
}) {
  return (
    <div className="search-chips">
      <span>{title}</span>
      {items.length === 0 ? (
        <p className="profile-name-hint">{empty}</p>
      ) : (
        <div className="search-chips__list">
          <button className={all ? 'search-chip search-chip--on' : 'search-chip'} onClick={onAll} type="button">{allLabel}</button>
          {items.map((item) => (
            <button
              className={!all && chosen.includes(item) ? 'search-chip search-chip--on' : 'search-chip'}
              key={item}
              onClick={() => onToggle(item)}
              type="button"
            >
              {item}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// Um cartão de contagens: uma barra por valor (a % é do total do cartão) e o
// "Não informado" no fim.
function CountCard({ title, rows, of, missing, note, active, onPick }: {
  title: string
  rows: Array<{ value: string; label: string; count: number; detail?: string }>
  of: number
  missing?: number
  note?: string
  active?: string
  onPick?: (value: string) => void
}) {
  const all = missing !== undefined ? [...rows, { value: '', label: 'Não informado', count: missing }] : rows
  return (
    <div className="search-card">
      <h2>{title}</h2>
      {note && <p className="profile-name-hint">{note}</p>}
      {rows.length === 0 && missing === undefined && <p className="profile-name-hint">Nenhuma imagem.</p>}
      <ul className="search-bars">
        {all.map((row) => {
          const clickable = Boolean(onPick && row.value)
          const className = [
            'search-bar',
            clickable ? 'search-bar--clickable' : '',
            active && row.value === active ? 'search-bar--active' : '',
            row.value === '' ? 'search-bar--missing' : '',
          ].filter(Boolean).join(' ')
          const content = (
            <>
              <span className="search-bar__label">{row.label}</span>
              <span className="search-bar__track"><span className="search-bar__fill" style={{ width: pct(row.count, of) }} /></span>
              <span className="search-bar__count">
                {fmt(row.count)} <small>{pct(row.count, of)}</small>
                {'detail' in row && row.detail && <small className="search-bar__detail">{row.detail}</small>}
              </span>
            </>
          )
          return (
            <li key={row.value || '__missing'}>
              {clickable ? (
                <button className={className} onClick={() => onPick?.(row.value)} title={active === row.value ? 'Tirar o filtro' : 'Filtrar por este valor'} type="button">
                  {content}
                </button>
              ) : (
                <div className={className}>{content}</div>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
