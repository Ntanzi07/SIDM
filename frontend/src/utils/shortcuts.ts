// Os atalhos de teclado: as ações de cada tela, a tecla padrão e como uma
// tecla vira texto ("ctrl+enter", "shift+delete", "f"). Cada pessoa pode
// trocar as dela (Atalhos, no menu da conta); ficam na conta, no servidor.

export type ShortcutAction = {
  id: string // "annotation.next": o que fica gravado na conta
  label: string
  help?: string
  key: string // o padrão
  group?: string // para agrupar na ajuda ("Navegar", "Anotar"...)
}

export type ShortcutScope = { id: string; label: string; actions: ShortcutAction[] }

export const SHORTCUT_SCOPES: ShortcutScope[] = [
  {
    id: 'annotation',
    label: 'Anotação',
    actions: [
      { id: 'annotation.previous', group: 'Navegar', label: 'Imagem anterior', key: 'd' },
      { id: 'annotation.next', group: 'Navegar', label: 'Próxima imagem', help: 'Pula a imagem (ela volta para o fim da fila) ou, revendo, vai para a seguinte', key: 'f' },
      { id: 'annotation.save', group: 'Anotar', label: 'Salvar anotação', help: 'Confere e verifica a imagem', key: 'ctrl+enter' },
      { id: 'annotation.add_object', group: 'Anotar', label: 'Adicionar objeto', help: 'O primeiro tipo da lista (ex.: veículo)', key: 'a' },
      { id: 'annotation.focus_first', group: 'Anotar', label: 'Ir para o primeiro campo', help: 'O primeiro campo do último objeto (ex.: a placa)', key: 'p' },
      { id: 'annotation.toggle_boxes', group: 'Caixas', label: 'Mostrar/esconder as caixas', key: 'b' },
      { id: 'annotation.next_box', group: 'Caixas', label: 'Próxima caixa', help: 'Seleciona e mostra a caixa na imagem', key: 'e' },
      { id: 'annotation.previous_box', group: 'Caixas', label: 'Caixa anterior', key: 'q' },
      { id: 'annotation.delete_image', group: 'Outros', label: 'Apagar a imagem', help: 'Abre a confirmação (vai para a lixeira)', key: 'shift+delete' },
      { id: 'annotation.help', group: 'Outros', label: 'Ver os atalhos', key: '?' },
    ],
  },
]

export const ALL_ACTIONS = SHORTCUT_SCOPES.flatMap((scope) => scope.actions)

// "none" na conta: a ação fica sem atalho.
export const NO_KEY = 'none'

// As teclas de cada ação: o padrão, trocado pelo que a pessoa gravou.
export function effectiveShortcuts(saved: Record<string, string> | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  for (const action of ALL_ACTIONS) out[action.id] = saved?.[action.id] ?? action.key
  return out
}

const NAMES: Record<string, string> = { ' ': 'space', escape: 'esc', arrowleft: 'left', arrowright: 'right', arrowup: 'up', arrowdown: 'down' }

// A tecla de um evento, no formato gravado. Um caractere (letra, "?", "1")
// vale como veio (o shift já está nele); tecla com nome leva o shift.
export function comboOf(event: KeyboardEvent): string | null {
  const raw = event.key.toLowerCase()
  if (['control', 'shift', 'alt', 'meta', 'dead', 'unidentified'].includes(raw)) return null
  const key = NAMES[raw] ?? raw
  const parts: string[] = []
  if (event.ctrlKey || event.metaKey) parts.push('ctrl')
  if (event.altKey) parts.push('alt')
  if (event.shiftKey && key.length > 1) parts.push('shift')
  parts.push(key)
  return parts.join('+')
}

// Como mostrar na tela: "Ctrl + Enter", "Shift + Delete", "F".
export function formatCombo(combo: string): string {
  if (!combo || combo === NO_KEY) return '—'
  return combo.split('+').map((part) => (part.length === 1 ? part.toUpperCase() : part[0].toUpperCase() + part.slice(1))).join(' + ')
}

// Digitando num campo: só valem atalhos com Ctrl/Alt (senão "f" não escreve).
export function typingIn(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}
