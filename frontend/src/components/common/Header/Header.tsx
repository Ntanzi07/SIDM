import { NavLink } from 'react-router-dom'
import './Header.css'

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
          to="/exportar"
        >
          Exportar
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
      <div className="app-header__user">Desconhecido</div>
    </header>
  )
}