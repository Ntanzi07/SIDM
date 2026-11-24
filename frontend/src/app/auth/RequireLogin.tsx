import { useEffect, useState } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { getMe } from '../../services/api'
import { setAccount, useAccount } from '../../services/user'
import { PageLoader } from '../../components/PageLoader/PageLoader'

// Toda tela, menos o login, pede uma sessão. Ao abrir o app, pergunta ao
// servidor quem está logado (o cookie é HttpOnly: só ele sabe).
export function RequireLogin() {
  const account = useAccount()
  const location = useLocation()
  const [failed, setFailed] = useState('')

  useEffect(() => {
    if (account !== undefined) return
    getMe()
      .then(setAccount)
      .catch((error: unknown) => setFailed(error instanceof Error ? error.message : 'O servidor não respondeu.'))
  }, [account])

  if (failed) {
    return (
      <div className="auth-wait">
        <p className="form-error">Não foi possível falar com o servidor: {failed}</p>
        <button className="button button--secondary" onClick={() => window.location.reload()} type="button">Tentar de novo</button>
      </div>
    )
  }
  if (account === undefined) return <PageLoader label="Entrando" />
  if (account === null) return <Navigate replace state={{ from: location.pathname + location.search }} to="/login" />
  return <Outlet />
}

// Só admin: o Configurar e as contas. Para quem não é, avisa em vez de
// mostrar uma tela onde nada funciona.
export function RequireAdmin() {
  const account = useAccount()
  if (account?.role !== 'admin') {
    return (
      <section className="package-page">
        <div className="page-heading">
          <div>
            <h1>Só para admin</h1>
            <p>Esta tela muda configurações de todo mundo. Peça para um admin, se precisar de algo aqui.</p>
          </div>
        </div>
      </section>
    )
  }
  return <Outlet />
}
