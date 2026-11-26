import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { NavLink, useLocation } from 'react-router-dom'
import { changePassword, listAccounts, logout } from '../../../services/api'
import { setPendingCount, useAccount, usePendingCount } from '../../../services/user'

// A conta logada, à direita do menu: o nome, e ao clicar trocar a senha,
// as contas (admin, com quantas esperam aprovação) e sair.
export function UserMenu() {
  const account = useAccount()
  const { pathname } = useLocation()
  const [open, setOpen] = useState(false)
  const [changing, setChanging] = useState(false)
  const pending = usePendingCount()
  const ref = useRef<HTMLDivElement>(null)
  const admin = account?.role === 'admin'

  // admin: quantos cadastros esperam (confere de novo a cada tela)
  useEffect(() => {
    if (!admin) return
    let alive = true
    listAccounts()
      .then((list) => { if (alive) setPendingCount(list.filter((item) => item.status === 'pending').length) })
      .catch(() => { /* sem o número, sem o selo */ })
    return () => { alive = false }
  }, [admin, pathname])

  useEffect(() => {
    if (!open) return
    const onDown = (event: MouseEvent) => { if (!ref.current?.contains(event.target as Node)) setOpen(false) }
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') setOpen(false) }
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', onDown)
      window.removeEventListener('keydown', onKey)
    }
  }, [open])

  if (!account) return null
  const first = account.name.trim().split(/\s+/)[0] || account.username
  return (
    <div className="user-menu" ref={ref}>
      <button
        aria-expanded={open}
        aria-haspopup="menu"
        className="app-header__user app-header__user-button"
        onClick={() => setOpen((value) => !value)}
        type="button"
      >
        {first}
        {admin && pending > 0 && <span aria-label={`${pending} cadastro(s) esperando`} className="user-menu__badge">{pending}</span>}
      </button>
      {open && (
        <div className="user-menu__panel" role="menu">
          <div className="user-menu__who">
            <strong>{account.name}</strong>
            <span>{account.username} · {admin ? 'admin' : 'usuário'}</span>
          </div>
          {admin && (
            <NavLink className="user-menu__item" onClick={() => setOpen(false)} role="menuitem" to="/configurar?aba=usuarios">
              Usuários
              {pending > 0 && <span className="user-menu__count">{pending} esperando</span>}
            </NavLink>
          )}
          <NavLink className="user-menu__item" onClick={() => setOpen(false)} role="menuitem" to="/configurar?aba=atalhos">Atalhos de teclado</NavLink>
          <button className="user-menu__item" onClick={() => { setOpen(false); setChanging(true) }} role="menuitem" type="button">Trocar a senha</button>
          <button className="user-menu__item user-menu__item--out" onClick={() => void logout()} role="menuitem" type="button">Sair</button>
        </div>
      )}
      {changing && <ChangePasswordModal onClose={() => setChanging(false)} />}
    </div>
  )
}

export function ChangePasswordModal({ onClose }: { onClose: () => void }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (next !== confirm) {
      setError('As duas senhas novas não são iguais.')
      return
    }
    setBusy(true)
    setError('')
    try {
      await changePassword(current, next)
      setDone(true)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível trocar a senha.')
    } finally {
      setBusy(false)
    }
  }

  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={() => { if (!busy) onClose() }} role="dialog">
      <div className="modal-dialog" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Trocar a senha</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy} onClick={onClose} type="button">✕</button>
        </div>
        {done ? (
          <>
            <div className="modal-body">
              <p className="password-form__done">Senha trocada. Se você estava logado em outro computador, lá vai pedir para entrar de novo.</p>
            </div>
            <div className="modal-actions">
              <button className="button button--primary" onClick={onClose} type="button">Fechar</button>
            </div>
          </>
        ) : (
          <form onSubmit={(event) => void submit(event)}>
            <div className="modal-body password-form">
              <label>
                <span>Senha atual</span>
                <input autoComplete="current-password" autoFocus onChange={(event) => setCurrent(event.target.value)} required type="password" value={current} />
              </label>
              <label>
                <span>Senha nova</span>
                <input autoComplete="new-password" minLength={8} onChange={(event) => setNext(event.target.value)} required type="password" value={next} />
                <small>Pelo menos 8 caracteres.</small>
              </label>
              <label>
                <span>Repita a senha nova</span>
                <input autoComplete="new-password" onChange={(event) => setConfirm(event.target.value)} required type="password" value={confirm} />
              </label>
              {error && <p className="form-error" role="alert">{error}</p>}
            </div>
            <div className="modal-actions">
              <button className="button button--secondary" disabled={busy} onClick={onClose} type="button">Cancelar</button>
              <button className="button button--primary" disabled={busy} type="submit">{busy ? 'Trocando...' : 'Trocar'}</button>
            </div>
          </form>
        )}
      </div>
    </div>,
    document.body,
  )
}
