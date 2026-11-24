import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { login, register } from '../../services/api'
import { useAccount } from '../../services/user'
import './Login.css'

type Mode = 'login' | 'register'

// Entrar ou pedir uma conta. Quem se cadastra espera um admin aprovar.
export function LoginPage() {
  const account = useAccount()
  const location = useLocation()
  const [mode, setMode] = useState<Mode>('login')
  const [username, setUsername] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [sent, setSent] = useState('')

  const from = (location.state as { from?: string } | null)?.from ?? '/'
  if (account) return <Navigate replace to={from === '/login' ? '/' : from} />

  function switchTo(next: Mode) {
    setMode(next)
    setError('')
    setPassword('')
    setConfirm('')
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    if (mode === 'register' && password !== confirm) {
      setError('As duas senhas não são iguais.')
      return
    }
    setBusy(true)
    try {
      if (mode === 'login') {
        await login(username, password)
        return // a conta mudou: o Navigate acima leva para a tela
      }
      const created = await register({ username, name, password })
      setSent(created.username)
      switchTo('login')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível concluir.')
    } finally {
      setBusy(false)
    }
  }

  const registering = mode === 'register'
  return (
    <main className="login-page">
      <form className="login-card" onSubmit={(event) => void submit(event)}>
        <span className="login-card__brand">SIDM</span>
        <h1>{registering ? 'Pedir uma conta' : 'Entrar'}</h1>
        <p className="login-card__hint">
          {registering
            ? 'Um admin precisa aprovar antes do primeiro acesso.'
            : 'Controle e anotação de imagens.'}
        </p>

        {sent && !registering && (
          <p className="login-card__sent" role="status">
            Cadastro de <strong>{sent}</strong> enviado. Assim que um admin aprovar, é só entrar aqui.
          </p>
        )}

        {registering && (
          <label>
            <span>Seu nome</span>
            <input autoComplete="name" onChange={(event) => setName(event.target.value)} placeholder="Maria Silva" required value={name} />
          </label>
        )}
        <label>
          <span>Usuário</span>
          <input
            autoCapitalize="none"
            autoComplete="username"
            autoFocus
            onChange={(event) => setUsername(event.target.value)}
            placeholder={registering ? 'maria.silva' : ''}
            required
            spellCheck={false}
            value={username}
          />
          {registering && <small>Letras minúsculas, números, ponto ou hífen. É o nome que fica no histórico das imagens.</small>}
        </label>
        <label>
          <span>Senha</span>
          <input
            autoComplete={registering ? 'new-password' : 'current-password'}
            minLength={registering ? 8 : undefined}
            onChange={(event) => setPassword(event.target.value)}
            required
            type="password"
            value={password}
          />
          {registering && <small>Pelo menos 8 caracteres.</small>}
        </label>
        {registering && (
          <label>
            <span>Repita a senha</span>
            <input autoComplete="new-password" onChange={(event) => setConfirm(event.target.value)} required type="password" value={confirm} />
          </label>
        )}

        {error && <p className="form-error" role="alert">{error}</p>}

        <button className="button button--primary login-card__submit" disabled={busy} type="submit">
          {busy ? 'Aguarde...' : registering ? 'Enviar cadastro' : 'Entrar'}
        </button>
        <p className="login-card__switch">
          {registering ? 'Já tem conta?' : 'Primeira vez aqui?'}{' '}
          <button className="login-card__link" onClick={() => switchTo(registering ? 'login' : 'register')} type="button">
            {registering ? 'Entrar' : 'Pedir uma conta'}
          </button>
        </p>
      </form>
    </main>
  )
}
