import type { ReactNode } from 'react'

// As peças das telas do Configurar: o cabeçalho de cada tela, os grupos e
// as linhas (nome e ajuda à esquerda, o controle à direita), como nas
// configurações do GitHub e do Vercel. Um grupo é um painel com linhas
// separadas por filetes, não um cartão por configuração.

export function SettingsHeader({ title, description, actions }: { title: string; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="set-header">
      <div>
        <h2>{title}</h2>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="set-header__actions">{actions}</div>}
    </header>
  )
}

// tone="danger": a zona de perigo (sempre a última da tela).
export function SettingsGroup({ title, description, tone, children }: { title?: string; description?: ReactNode; tone?: 'danger'; children: ReactNode }) {
  return (
    <section className={tone === 'danger' ? 'set-group set-group--danger' : 'set-group'}>
      {(title || description) && (
        <div className="set-group__head">
          {title && <h3>{title}</h3>}
          {description && <p>{description}</p>}
        </div>
      )}
      <div className="set-group__panel">{children}</div>
    </section>
  )
}

// Uma configuração. stack: o controle vai embaixo (listas, medidores).
export function SettingRow({ label, help, htmlFor, stack, children }: { label: ReactNode; help?: ReactNode; htmlFor?: string; stack?: boolean; children?: ReactNode }) {
  return (
    <div className={stack ? 'set-row set-row--stack' : 'set-row'}>
      <div className="set-row__text">
        {htmlFor ? <label className="set-row__label" htmlFor={htmlFor}>{label}</label> : <span className="set-row__label">{label}</span>}
        {help && <span className="set-row__help">{help}</span>}
      </div>
      {children !== undefined && <div className="set-row__control">{children}</div>}
    </div>
  )
}
