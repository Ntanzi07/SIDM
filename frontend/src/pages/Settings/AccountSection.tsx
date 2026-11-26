import { useEffect, useState } from 'react'
import { ChangePasswordModal } from '../../components/common/Header/UserMenu'
import { getTheme, setTheme, subscribeTheme, type Theme } from '../../services/theme'
import { useAccount } from '../../services/user'
import { SettingRow, SettingsGroup, SettingsHeader } from './SettingsUI'

// Conta: quem sou, a aparência e a senha (de todo mundo).
export function AccountSection() {
  const account = useAccount()
  const [theme, setCurrent] = useState<Theme>(getTheme())
  const [changing, setChanging] = useState(false)
  useEffect(() => subscribeTheme(setCurrent), [])
  if (!account) return null

  return (
    <>
      <SettingsHeader description="Seus dados de acesso e como o SIDM aparece para você." title="Conta" />

      <SettingsGroup title="Perfil">
        <SettingRow help="Como a equipe vê você." label="Nome"><span className="set-value">{account.name || '—'}</span></SettingRow>
        <SettingRow help="Usado para entrar." label="E-mail"><span className="set-value">{account.email || 'sem e-mail'}</span></SettingRow>
        <SettingRow help="Fica gravado no histórico das imagens que você anota." label="Usuário"><code className="set-code">{account.username}</code></SettingRow>
        <SettingRow help={account.role === 'admin' ? 'Pode apagar, configurar e aprovar contas.' : 'Anota, importa, exporta e cria versões.'} label="Acesso">
          <span className={account.role === 'admin' ? 'set-pill set-pill--accent' : 'set-pill'}>{account.role === 'admin' ? 'Admin' : 'Usuário'}</span>
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup description="Fica salva neste navegador." title="Aparência">
        <SettingRow label="Tema" stack>
          <div aria-label="Tema" className="theme-choices" role="radiogroup">
            {(['light', 'dark'] as const).map((option) => (
              <button
                aria-checked={theme === option}
                className={theme === option ? 'theme-choice theme-choice--on' : 'theme-choice'}
                key={option}
                onClick={() => setTheme(option)}
                role="radio"
                type="button"
              >
                <span className={`theme-choice__preview theme-choice__preview--${option}`}>
                  <span /><span /><span />
                </span>
                {option === 'light' ? 'Claro' : 'Escuro'}
              </button>
            ))}
          </div>
        </SettingRow>
      </SettingsGroup>

      <SettingsGroup title="Segurança">
        <SettingRow help="Ao trocar, você sai dos outros computadores." label="Senha">
          <button className="button button--secondary button--sm" onClick={() => setChanging(true)} type="button">Trocar senha</button>
        </SettingRow>
      </SettingsGroup>
      {changing && <ChangePasswordModal onClose={() => setChanging(false)} />}
    </>
  )
}
