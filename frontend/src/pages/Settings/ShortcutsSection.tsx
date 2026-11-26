import { useEffect, useState } from 'react'
import { saveShortcuts } from '../../services/api'
import { notify } from '../../services/toast'
import { useAccount } from '../../services/user'
import { comboOf, effectiveShortcuts, formatCombo, NO_KEY, SHORTCUT_SCOPES } from '../../utils/shortcuts'
import { SettingRow, SettingsGroup, SettingsHeader } from './SettingsUI'
import './Shortcuts.css'

// Os atalhos de teclado da pessoa logada: cada um é da conta (vale em
// qualquer computador). "Mudar" espera a próxima tecla; Esc cancela.
export function ShortcutsSection() {
  const account = useAccount()
  const [draft, setDraft] = useState<Record<string, string>>(() => effectiveShortcuts(account?.shortcuts))
  const [capturing, setCapturing] = useState('')
  const [saving, setSaving] = useState(false)
  const saved = effectiveShortcuts(account?.shortcuts)
  const dirty = JSON.stringify(saved) !== JSON.stringify(draft)

  useEffect(() => {
    if (!capturing) return
    const onKey = (event: KeyboardEvent) => {
      event.preventDefault()
      event.stopPropagation()
      if (event.key === 'Escape') {
        setCapturing('')
        return
      }
      const combo = comboOf(event)
      if (!combo) return // só Ctrl/Shift: espera a tecla
      setDraft((current) => ({ ...current, [capturing]: combo }))
      setCapturing('')
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [capturing])

  // a mesma tecla em duas ações da mesma tela
  const conflicts = new Set<string>()
  for (const scope of SHORTCUT_SCOPES) {
    const seen = new Map<string, string>()
    for (const action of scope.actions) {
      const key = draft[action.id]
      if (!key || key === NO_KEY) continue
      const other = seen.get(key)
      if (other) { conflicts.add(action.id); conflicts.add(other) }
      seen.set(key, action.id)
    }
  }

  // O botão de salvar nunca fica desabilitado (vectorlab:forms): sem mudança,
  // diz isso; com teclas repetidas, leva até a primeira e diz o que fazer.
  function trySave() {
    if (conflicts.size > 0) {
      const first = SHORTCUT_SCOPES.flatMap((scope) => scope.actions).find((action) => conflicts.has(action.id))
      const button = first && document.getElementById(`shortcut-${first.id}`)
      button?.scrollIntoView({ block: 'center' })
      button?.focus()
      notify('Escolha outra tecla para uma das ações marcadas: a mesma tecla está em duas ações da mesma tela.', 'error')
      return
    }
    if (!dirty) {
      notify('Nenhuma alteração para salvar.', 'info')
      return
    }
    void save(draft)
  }

  async function save(next: Record<string, string>) {
    setSaving(true)
    try {
      // só o que é diferente do padrão vai para a conta
      const changed: Record<string, string> = {}
      for (const scope of SHORTCUT_SCOPES) {
        for (const action of scope.actions) {
          if (next[action.id] !== action.key) changed[action.id] = next[action.id] || NO_KEY
        }
      }
      const account = await saveShortcuts(changed)
      setDraft(effectiveShortcuts(account.shortcuts))
      notify('Atalhos salvos.', 'success')
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível salvar os atalhos.', 'error')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <SettingsHeader
        description="São da sua conta e valem em qualquer computador. Digitando num campo, só funcionam os atalhos com Ctrl ou Alt; Esc sai do campo."
        title="Atalhos de teclado"
      />

      {SHORTCUT_SCOPES.map((scope) => (
        <SettingsGroup key={scope.id} title={scope.label}>
          {scope.actions.map((action) => {
            const key = draft[action.id]
            const listening = capturing === action.id
            const conflict = conflicts.has(action.id)
            const shown = listening ? 'Aperte…' : formatCombo(key)
            return (
              <SettingRow
                help={listening ? 'Aperte a tecla nova. Esc cancela.' : conflict ? 'Essa tecla já está em outra ação desta tela: escolha outra.' : action.help}
                key={action.id}
                label={action.label}
              >
                <div className={conflict ? 'shortcut-control shortcut-control--conflict' : 'shortcut-control'}>
                  {/* o nome acessível começa pelo texto visível (WCAG 2.5.3): quem
                      usa comando de voz diz o que vê ("clique em D") */}
                  <button
                    aria-label={`${shown}: mudar o atalho de ${action.label}`}
                    className={listening ? 'shortcut-key shortcut-key--listening' : 'shortcut-key'}
                    disabled={saving}
                    id={`shortcut-${action.id}`}
                    onClick={() => setCapturing(listening ? '' : action.id)}
                    title={listening ? 'Esc cancela' : 'Clique para mudar'}
                    type="button"
                  >
                    {shown}
                  </button>
                  {key !== action.key ? (
                    <button className="set-link" disabled={saving} onClick={() => setDraft({ ...draft, [action.id]: action.key })} title={`Padrão: ${formatCombo(action.key)}`} type="button">Restaurar</button>
                  ) : key !== NO_KEY ? (
                    <button className="set-link" disabled={saving} onClick={() => setDraft({ ...draft, [action.id]: NO_KEY })} type="button">Remover</button>
                  ) : null}
                </div>
              </SettingRow>
            )
          })}
        </SettingsGroup>
      ))}

      <div className={dirty ? 'set-savebar set-savebar--dirty' : 'set-savebar'} role="status">
        <span>{conflicts.size > 0 ? 'Resolva as teclas repetidas para salvar.' : dirty ? 'Alterações não salvas.' : 'Tudo salvo.'}</span>
        <div>
          {dirty && <button className="button button--secondary button--sm" disabled={saving} onClick={() => setDraft(saved)} type="button">Descartar</button>}
          {/* só muda o rascunho: dá para conferir e Descartar antes de salvar */}
          <button className="button button--secondary button--sm" disabled={saving} onClick={() => setDraft(effectiveShortcuts(undefined))} type="button">Restaurar padrões</button>
          <button aria-busy={saving || undefined} className="button button--primary button--sm" disabled={saving} onClick={trySave} type="button">
            {saving ? 'Salvando…' : 'Salvar atalhos'}
          </button>
        </div>
      </div>
    </>
  )
}
