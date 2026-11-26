import { useEffect, useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { getTheme, setTheme, subscribeTheme } from '../../../services/theme'
import { useIsAdmin } from '../../../services/user'
import { UserMenu } from './UserMenu'
import './Header.css'

// Alterna entre claro e escuro. O ícone mostra o tema para onde vai.
function ThemeToggle() {
  const [theme, setCurrent] = useState(getTheme())
  useEffect(() => subscribeTheme(setCurrent), [])
  const next = theme === 'dark' ? 'light' : 'dark'
  const label = next === 'light' ? 'Mudar para o modo claro' : 'Mudar para o modo escuro'
  return (
    <button aria-label={label} className="app-header__theme" onClick={() => setTheme(next)} title={label} type="button">
      <svg aria-hidden="true" fill="none" height="16" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" viewBox="0 0 24 24" width="16">
        {next === 'light' ? (
          <>
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
          </>
        ) : (
          <path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5Z" />
        )}
      </svg>
    </button>
  )
}

// O menu: "Pacotes" fica ativo também dentro de um pacote (/pacotes/...).
const LINKS = [
  { to: '/', label: 'Pacotes', also: '/pacotes' },
  { to: '/tarefas', label: 'Tarefas' },
  { to: '/processar', label: 'Processar', admin: true },
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/exportar', label: 'Exportar' },
  { to: '/configurar', label: 'Configurar' }, // todos: a própria conta; o resto é de admin
]

export function Header() {
  const { pathname } = useLocation()
  const admin = useIsAdmin()
  const active = (link: (typeof LINKS)[number]) =>
    link.to === '/' ? pathname === '/' || pathname.startsWith(link.also ?? '') : pathname.startsWith(link.to)

  return (
    <header className="app-header">
      <NavLink className="app-header__brand" to="/">SIDM</NavLink>
      <nav aria-label="Navegação principal" className="app-header__nav">
        {LINKS.filter((link) => admin || !link.admin).map((link) => (
          <NavLink
            aria-current={active(link) ? 'page' : undefined}
            className={active(link) ? 'app-header__link app-header__link--active' : 'app-header__link'}
            key={link.to}
            to={link.to}
          >
            {link.label}
          </NavLink>
        ))}
      </nav>
      <ThemeToggle />
      <UserMenu />
    </header>
  )
}
