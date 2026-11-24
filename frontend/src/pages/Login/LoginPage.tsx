import { useState } from 'react'
import type { FormEvent } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { login, register } from '../../services/api'
import { useAccount } from '../../services/user'
import './Login.css'

type Mode = 'login' | 'register'

// Entrar ou pedir uma conta. À esquerda a marca sobre o gradiente; à
// direita, num painel de vidro, o formulário. Entra-se pelo e-mail; quem se
// cadastra espera um admin aprovar.
export function LoginPage() {
  const account = useAccount()
  const location = useLocation()
  const [mode, setMode] = useState<Mode>('login')
  const [email, setEmail] = useState('')
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
        await login(email, password)
        return // a conta mudou: o Navigate acima leva para a tela
      }
      const created = await register({ email, name, password })
      setSent(created.email ?? email)
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
      <div aria-hidden="true" className="login-page__glow">
        <span className="login-page__orb login-page__orb--1" />
        <span className="login-page__orb login-page__orb--2" />
        <span className="login-page__orb login-page__orb--3" />
      </div>

      <section className="login-hero">
        <span className="login-hero__brand">SIDM</span>
        <p className="login-hero__line">Controle e anotação de imagens de veículos.</p>
        <span className="login-hero__foot">Splice · uso interno</span>
      </section>

      <section className="login-glass">
        <form className="login-form" onSubmit={(event) => void submit(event)}>
          <div className="login-form__head">
            <h1>{registering ? 'Pedir uma conta' : 'Entrar'}</h1>
            <p>{registering ? 'Um admin aprova antes do primeiro acesso.' : 'Use o seu e-mail e a sua senha.'}</p>
          </div>

          {sent && !registering && (
            <p className="login-form__sent" role="status">
              Cadastro de <strong>{sent}</strong> enviado. Assim que um admin aprovar, é só entrar.
            </p>
          )}

          {registering && (
            <label>
              <span>Seu nome</span>
              <input autoComplete="name" onChange={(event) => setName(event.target.value)} placeholder="Maria Silva" required value={name} />
            </label>
          )}
          <label>
            <span>E-mail</span>
            <input
              autoCapitalize="none"
              autoComplete={registering ? 'email' : 'username'}
              autoFocus
              inputMode="email"
              onChange={(event) => setEmail(event.target.value)}
              placeholder="voce@empresa.com.br"
              required
              spellCheck={false}
              type={registering ? 'email' : 'text'}
              value={email}
            />
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

          <button className="button button--primary login-form__submit" disabled={busy} type="submit">
            {busy ? 'Aguarde...' : registering ? 'Enviar cadastro' : 'Entrar'}
          </button>
          <p className="login-form__switch">
            {registering ? 'Já tem conta?' : 'Primeira vez aqui?'}{' '}
            <button className="login-form__link" onClick={() => switchTo(registering ? 'login' : 'register')} type="button">
              {registering ? 'Entrar' : 'Pedir uma conta'}
            </button>
          </p>
        </form>
      </section>
    </main>
  )
}
