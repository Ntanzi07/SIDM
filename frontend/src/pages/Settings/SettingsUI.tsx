import type { ReactNode } from 'react'
import { HelpTip } from '../../components/HelpTip/HelpTip'

// As peças das telas do Configurar: o cabeçalho de cada tela, os grupos e
// as linhas (nome e ajuda à esquerda, o controle à direita), como nas
// configurações do GitHub e do Vercel. Um grupo é um painel com linhas
// separadas por filetes, não um cartão por configuração.
//
// info (opcional): a explicação longa vai no ícone (?) ao lado do título, em
// vez de um parágrafo. Só para telas com muito texto; texto curto fica à
// vista em description/help.

export function SettingsHeader({ title, description, info, actions }: { title: string; description?: ReactNode; info?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="set-header">
      <div>
        <h2>{title}{info && <HelpTip label={title}>{info}</HelpTip>}</h2>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="set-header__actions">{actions}</div>}
    </header>
  )
}

// tone="danger": a zona de perigo (sempre a última da tela).
export function SettingsGroup({ title, description, info, tone, children }: { title?: string; description?: ReactNode; info?: ReactNode; tone?: 'danger'; children: ReactNode }) {
  return (
    <section className={tone === 'danger' ? 'set-group set-group--danger' : 'set-group'}>
      {(title || description) && (
        <div className="set-group__head">
          {title && <h3>{title}{info && <HelpTip label={title}>{info}</HelpTip>}</h3>}
          {description && <p>{description}</p>}
        </div>
      )}
      <div className="set-group__panel">{children}</div>
    </section>
  )
}

// Uma configuração. stack: o controle vai embaixo (listas, medidores).
export function SettingRow({ label, help, info, htmlFor, stack, children }: { label: ReactNode; help?: ReactNode; info?: ReactNode; htmlFor?: string; stack?: boolean; children?: ReactNode }) {
  return (
    <div className={stack ? 'set-row set-row--stack' : 'set-row'}>
      <div className="set-row__text">
        <span className="set-row__name">
          {htmlFor ? <label className="set-row__label" htmlFor={htmlFor}>{label}</label> : <span className="set-row__label">{label}</span>}
          {info && <HelpTip label={typeof label === 'string' ? label : undefined}>{info}</HelpTip>}
        </span>
        {help && <span className="set-row__help">{help}</span>}
      </div>
      {children !== undefined && <div className="set-row__control">{children}</div>}
    </div>
  )
}
