import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { NavLink } from 'react-router-dom'
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

export function Header() {
  return (
    <header className="app-header">
      <div className="app-header__brand">SIDM</div>
      <nav aria-label="Navegação principal">
        <NavLink
          className={({ isActive }) =>
            isActive ? 'app-header__link app-header__link--active' : 'app-header__link'
          }
          to="/"
          end
        >
          Pacotes
        </NavLink>
        <NavLink
          className={({ isActive }) =>
            isActive ? 'app-header__link app-header__link--active' : 'app-header__link'
          }
          to="/tarefas"
        >
          Tarefas
        </NavLink>
        <NavLink
          className={({ isActive }) =>
            isActive ? 'app-header__link app-header__link--active' : 'app-header__link'
          }
          to="/exportar"
        >
          Exportar
        </NavLink>
        <NavLink
          className={({ isActive }) =>
            isActive ? 'app-header__link app-header__link--active' : 'app-header__link'
          }
          to="/pesquisa"
        >
          Pesquisa
        </NavLink>
        <NavLink
          className={({ isActive }) =>
            isActive ? 'app-header__link app-header__link--active' : 'app-header__link'
          }
          to="/configurar"
        >
          Configurar
        </NavLink>
      </nav>
      <UserName />
    </header>
  )
}