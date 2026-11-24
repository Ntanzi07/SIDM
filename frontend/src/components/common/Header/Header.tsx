import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { getUser, setUser, subscribeUser } from '../../../services/user'
import './Header.css'

// O nome vai no header X-User de cada pedido (histórico e fila).
function UserName() {
  const [user, setCurrent] = useState(getUser())
  const [editing, setEditing] = useState(!getUser())
  const [draft, setDraft] = useState(getUser())

  useEffect(() => subscribeUser(setCurrent), [])

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!draft.trim()) return
    setUser(draft)
    setEditing(false)
  }

  if (editing) {
    return (
      <form className="app-header__user" onSubmit={submit}>
        <input
          aria-label="Seu nome"
          autoFocus={!user}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="Seu nome"
          value={draft}
        />
        <button className="app-header__user-button" disabled={!draft.trim()} type="submit">OK</button>
      </form>
    )
  }
  return (
    <button
      className="app-header__user app-header__user-button"
      onClick={() => { setDraft(user); setEditing(true) }}
      title="Trocar o nome"
      type="button"
    >
      {user}
    </button>
  )
}

// O menu: "Pacotes" fica ativo também dentro de um pacote (/pacotes/...).
const LINKS = [
  { to: '/', label: 'Pacotes', also: '/pacotes' },
  { to: '/tarefas', label: 'Tarefas' },
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/exportar', label: 'Exportar' },
  { to: '/configurar', label: 'Configurar' },
]

export function Header() {
  const { pathname } = useLocation()
  const active = (link: (typeof LINKS)[number]) =>
    link.to === '/' ? pathname === '/' || pathname.startsWith(link.also ?? '') : pathname.startsWith(link.to)

  return (
    <header className="app-header">
      <NavLink className="app-header__brand" to="/">SIDM</NavLink>
      <nav aria-label="Navegação principal" className="app-header__nav">
        {LINKS.map((link) => (
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
      <UserName />
    </header>
  )
}
