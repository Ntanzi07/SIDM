import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { listAccounts, resetAccountPassword, updateAccount } from '../../services/api'
import { notify } from '../../services/toast'
import { useToastState } from '../../hooks/useToast'
import { setPendingCount, useAccount, type Account } from '../../services/user'
import { formatWhen } from '../Versions/format'
import { Bone, Skeleton } from '../../components/Skeleton/Skeleton'
import { SettingsHeader } from '../Settings/SettingsUI'
import './Users.css'

// As contas (só admin): aprovar quem pediu, trocar o papel, desativar e
// definir uma senha nova para quem esqueceu.
// embedded: dentro do Configurar (sem o título da página).
export function UsersPage({ embedded = false }: { embedded?: boolean } = {}) {
  const me = useAccount()
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  const [error, setError] = useToastState('error') // o erro vira balão (o padrão do site)
  const [busy, setBusy] = useState('')
  const [resetting, setResetting] = useState<Account | null>(null)

  // o selo do menu acompanha a lista (aprovar ou recusar tira da conta)
  useEffect(() => {
    if (accounts) setPendingCount(accounts.filter((item) => item.status === 'pending').length)
  }, [accounts])

  useEffect(() => {
    listAccounts()
      .then(setAccounts)
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as contas.'))
  }, [setError])

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
    { status: 'active', title: 'Contas ativas', hint: '' },
    { status: 'disabled', title: 'Contas desativadas', hint: 'Não entram mais. O nome continua no histórico das imagens.' },
  ]

  return (
    <section className={embedded ? 'users-page users-page--embedded' : 'package-page users-page'}>
      {embedded ? (
        <SettingsHeader description="Quem entra no SIDM. Admin apaga, configura e aprova contas; usuário anota, importa, exporta e cria versões." title="Usuários" />
      ) : (
        <div className="page-heading">
          <div>
            <h1>Usuários</h1>
            <p>Quem pode entrar no SIDM. Admin apaga, mexe no Configurar e aprova contas; usuário anota, importa, exporta e cria versões.</p>
          </div>
        </div>
      )}
      {accounts === null && !error && (
        <Skeleton label="Carregando as contas">
          <div className="users-group">
            <div className="users-group__head"><Bone h={14} w={130} /></div>
            <ul className="users-list">
              {[0, 1, 2].map((i) => (
                <li className="users-row" key={i}>
                  <div className="sk-title"><Bone h={15} w={[90, 120, 76][i]} /><Bone h={12} w={[190, 160, 210][i]} /></div>
                  <Bone h={12} w={170} />
                  <div className="sk-buttons"><Bone h={32} r={8} w={92} /><Bone h={32} r={8} w={104} /></div>
                </li>
              ))}
            </ul>
          </div>
        </Skeleton>
      )}
      {accounts && groups.map((group) => {
        const list = accounts.filter((item) => item.status === group.status)
        // grupo vazio não aparece (nem o de aprovação: sem pedido, não há o que fazer)
        if (list.length === 0) return null
        return (
          <div className="users-group" key={group.status}>
            <div className="users-group__head">
              <h3 className="users-group__title">{group.title} <span className="users-group__count">{list.length}</span></h3>
              {group.hint && <p>{group.hint}</p>}
            </div>
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
