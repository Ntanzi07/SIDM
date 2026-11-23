import { useEffect, useRef, useState } from 'react'
import type { Columns, MenuOption } from './columns'

// As peças da barra de cima das grades (Pacotes, Tarefas): a busca com a
// lupa, o menu "rótulo: valor ⌄" e o seletor de colunas. Os estilos estão
// em pages/Packages/Packages.css.

export function SearchIcon() {
  return (
    <svg aria-hidden="true" fill="none" height="16" viewBox="0 0 24 24" width="16">
      <circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="1.8" />
      <path d="m20 20-3.5-3.5" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" />
    </svg>
  )
}

export function SearchField({ id, value, onChange, placeholder }: { id: string; value: string; onChange: (value: string) => void; placeholder: string }) {
  return (
    <label className="search-field" htmlFor={id}>
      <SearchIcon />
      <input id={id} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} type="search" value={value} />
    </label>
  )
}

// Um menu "rótulo: valor ⌄" com as opções embaixo; fecha ao escolher ou ao
// clicar fora. marked: o filtro está aplicado (o rótulo fica em destaque).
export function Menu({ label, value, options, onChange, marked = false }: { label: string; value: string; options: MenuOption[]; onChange: (value: string) => void; marked?: boolean }) {
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

// Os botões de colunas (1 = lista).
export function GridPicker({ options, value, onChange }: { options: readonly Columns[]; value: Columns; onChange: (value: Columns) => void }) {
  return (
    <div aria-label="Colunas da grade" className="grid-picker" role="group">
      {options.map((option) => (
        <button
          aria-pressed={value === option}
          className={value === option ? 'grid-picker__option grid-picker__option--active' : 'grid-picker__option'}
          key={option}
          onClick={() => onChange(option)}
          title={option === 1 ? 'Lista' : `${option} colunas`}
          type="button"
        >
          <GridIcon columns={option} />
        </button>
      ))}
    </div>
  )
}
