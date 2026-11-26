import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { formatCombo, SHORTCUT_SCOPES } from '../../utils/shortcuts'

// A lista dos atalhos da anotação (tecla "?"), agrupada, com o link para
// trocar. Esc fecha.
export function ShortcutsHelp({ keys, onClose }: { keys: Record<string, string>; onClose: () => void }) {
  const navigate = useNavigate()
  const actions = SHORTCUT_SCOPES.find((s) => s.id === 'annotation')?.actions ?? []
  const groups = [...new Set(actions.map((a) => a.group ?? 'Outros'))]

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); onClose() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const Keys = ({ combo }: { combo: string }) => (
    <span className="shortcuts-help__keys">
      {formatCombo(combo).split(' + ').map((part, i) => (
        <span key={i}>{i > 0 && <span className="shortcuts-help__plus">+</span>}<kbd>{part}</kbd></span>
      ))}
    </span>
  )

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-dialog shortcuts-help" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <strong>Atalhos da anotação</strong>
          <button aria-label="Fechar" className="modal-close" onClick={onClose} type="button">✕</button>
        </div>
        <div className="modal-body shortcuts-help__body">
          {groups.map((group) => (
            <section className="shortcuts-help__group" key={group}>
              <h3>{group}</h3>
              <ul>
                {actions.filter((a) => (a.group ?? 'Outros') === group).map((action) => (
                  <li key={action.id}>
                    <span>{action.label}</span>
                    <Keys combo={keys[action.id]} />
                  </li>
                ))}
                {group === 'Outros' && (
                  <li>
                    <span>Sair de um campo</span>
                    <Keys combo="esc" />
                  </li>
                )}
              </ul>
            </section>
          ))}
          <p className="shortcuts-help__tip">
            Digitando num campo, as letras escrevem: só valem os atalhos com Ctrl ou Alt. Aperte <kbd>Esc</kbd> para sair do campo.
          </p>
        </div>
        <div className="modal-actions">
          <button className="button button--secondary" onClick={() => { onClose(); navigate('/atalhos') }} type="button">Trocar as teclas</button>
          <button className="button button--primary" onClick={onClose} type="button">Fechar</button>
        </div>
      </div>
    </div>
  )
}
