import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { listAccounts, resetAccountPassword, updateAccount } from '../../services/api'
import { Toast, type ToastNotice } from '../../components/Toast/Toast'
import { setPendingCount, useAccount, type Account } from '../../services/user'
import { formatWhen } from '../Versions/format'
import { PageLoader } from '../../components/PageLoader/PageLoader'
import './Users.css'

// As contas (só admin): aprovar quem pediu, trocar o papel, desativar e
// definir uma senha nova para quem esqueceu.
export function UsersPage() {
  const me = useAccount()
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [notice, setNotice] = useState<ToastNotice | null>(null)
  const [resetting, setResetting] = useState<Account | null>(null)

  // o selo do menu acompanha a lista (aprovar ou recusar tira da conta)
  useEffect(() => {
    if (accounts) setPendingCount(accounts.filter((item) => item.status === 'pending').length)
  }, [accounts])

  useEffect(() => {
    listAccounts()
      .then(setAccounts)
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as contas.'))
  }, [])

  // a key do aviso só precisa mudar a cada um
  const notify = (text: string) => setNotice((prev) => ({ id: (prev?.id ?? 0) + 1, text }))

  async function change(account: Account, patch: { status?: 'active' | 'disabled'; role?: 'admin' | 'user' }, done: string) {
    setBusy(account.username)
    setError('')
    try {
      const updated = await updateAccount(account.username, patch)
      setAccounts((list) => list?.map((item) => (item.username === updated.username ? updated : item)) ?? null)
      notify(done)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível alterar a conta.')
    } finally {
      setBusy('')
    }
  }

  const groups: Array<{ status: Account['status']; title: string; hint: string }> = [
    { status: 'pending', title: 'Esperando aprovação', hint: 'Pediram uma conta. Aprove quem você reconhece.' },
    { status: 'active', title: 'Ativas', hint: '' },
    { status: 'disabled', title: 'Desativadas', hint: 'Não entram mais. O nome continua no histórico das imagens.' },
  ]

  return (
    <section className="package-page users-page">
      <div className="page-heading">
        <div>
          <h1>Usuários</h1>
          <p>Quem pode entrar no SIDM. Admin apaga, mexe no Configurar e aprova contas; usuário anota, importa, exporta e cria versões.</p>
        </div>
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      {accounts === null && !error && <PageLoader label="Carregando as contas" />}
      {accounts && groups.map((group) => {
        const list = accounts.filter((item) => item.status === group.status)
        if (list.length === 0 && group.status !== 'pending') return null
        return (
          <div className="users-group" key={group.status}>
            <div className="users-group__head">
              <span className="eyebrow">{group.title} · {list.length}</span>
              {group.hint && <p>{group.hint}</p>}
            </div>
            {list.length === 0 && <p className="users-empty">Ninguém esperando.</p>}
            <ul className="users-list">
              {list.map((account) => {
                const self = account.username === me?.username
                const working = busy === account.username
                return (
                  <li className="users-row" key={account.username}>
                    <div className="users-row__who">
                      <strong>{account.name}{self && <span className="users-row__you"> (você)</span>}</strong>
                      <span>{account.email ? `${account.email} · ` : ''}{account.username}</span>
                    </div>
                    <div className="users-row__when">
                      {account.status === 'pending'
                        ? `pediu em ${formatWhen(account.created_at)}`
                        : account.last_login ? `último acesso ${formatWhen(account.last_login)}` : 'ainda não entrou'}
                    </div>
                    <div className="users-row__actions">
                      {account.status === 'pending' && (
                        <>
                          <button className="button button--primary button--sm" disabled={working} onClick={() => void change(account, { status: 'active' }, `${account.username} aprovado.`)} type="button">Aprovar</button>
                          <button className="button button--secondary button--sm" disabled={working} onClick={() => void change(account, { status: 'disabled' }, `Cadastro de ${account.username} recusado.`)} type="button">Recusar</button>
                        </>
                      )}
                      {account.status === 'active' && (
                        <>
                          <select
                            aria-label={`Papel de ${account.username}`}
                            disabled={working}
                            onChange={(event) => void change(account, { role: event.target.value as Account['role'] }, `${account.username} agora é ${event.target.value === 'admin' ? 'admin' : 'usuário'}.`)}
                            value={account.role}
                          >
                            <option value="user">usuário</option>
                            <option value="admin">admin</option>
                          </select>
                          <button className="button button--secondary button--sm" disabled={working} onClick={() => setResetting(account)} type="button">Nova senha</button>
                          {!self && (
                            <button className="button button--danger button--sm" disabled={working} onClick={() => void change(account, { status: 'disabled' }, `${account.username} desativado.`)} type="button">Desativar</button>
                          )}
                        </>
                      )}
                      {account.status === 'disabled' && (
                        <button className="button button--secondary button--sm" disabled={working} onClick={() => void change(account, { status: 'active' }, `${account.username} reativado.`)} type="button">Reativar</button>
                      )}
                    </div>
                  </li>
                )
              })}
            </ul>
          </div>
        )
      })}
      {resetting && (
        <ResetPasswordModal
          account={resetting}
          onClose={() => setResetting(null)}
          onDone={() => { notify(`Senha nova de ${resetting.username} definida.`); setResetting(null) }}
        />
      )}
      <Toast notice={notice} />
    </section>
  )
}

function ResetPasswordModal({ account, onClose, onDone }: { account: Account; onClose: () => void; onDone: () => void }) {
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await resetAccountPassword(account.username, password)
      onDone()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível definir a senha.')
      setBusy(false)
    }
  }

  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={() => { if (!busy) onClose() }} role="dialog">
      <div className="modal-dialog" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Senha nova para {account.username}</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy} onClick={onClose} type="button">✕</button>
        </div>
        <form onSubmit={(event) => void submit(event)}>
          <div className="modal-body password-form">
            <p className="versions-hint">Passe a senha para a pessoa por um canal seguro e peça para ela trocar em “Trocar a senha”. Se estiver logada em algum computador, vai precisar entrar de novo.</p>
            <label>
              <span>Senha nova</span>
              <input autoComplete="new-password" autoFocus minLength={8} onChange={(event) => setPassword(event.target.value)} required type="password" value={password} />
              <small>Pelo menos 8 caracteres.</small>
            </label>
            {error && <p className="form-error" role="alert">{error}</p>}
          </div>
          <div className="modal-actions">
            <button className="button button--secondary" disabled={busy} onClick={onClose} type="button">Cancelar</button>
            <button className="button button--primary" disabled={busy || password.length < 8} type="submit">{busy ? 'Salvando...' : 'Definir'}</button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  )
}
