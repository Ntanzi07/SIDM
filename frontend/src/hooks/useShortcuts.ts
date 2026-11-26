import { useEffect, useRef } from 'react'
import { useAccount } from '../services/user'
import { comboOf, effectiveShortcuts, typingIn } from '../utils/shortcuts'

// Liga as ações de uma tela às teclas da pessoa logada. handlers: ação → o
// que fazer (pode mudar a cada render; o que vale é o da última). Esc num
// campo sai dele, para as letras voltarem a ser atalhos. enabled=false
// (um modal aberto) desliga tudo.
export function useShortcuts(handlers: Record<string, () => void>, enabled = true) {
  const account = useAccount()
  const keys = effectiveShortcuts(account?.shortcuts)
  const latest = useRef({ handlers, keys, enabled })
  useEffect(() => { latest.current = { handlers, keys, enabled } })

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const { handlers: current, keys: map, enabled: on } = latest.current
      if (!on || event.defaultPrevented) return
      const typing = typingIn(event.target)
      if (typing && event.key === 'Escape') {
        (event.target as HTMLElement).blur()
        return
      }
      const combo = comboOf(event)
      if (!combo) return
      if (typing && !event.ctrlKey && !event.metaKey && !event.altKey) return
      const action = Object.keys(current).find((id) => map[id] === combo)
      if (!action) return
      event.preventDefault()
      if (event.repeat) return
      current[action]()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return keys
}
