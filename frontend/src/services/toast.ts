// Os balões temporários do site (canto inferior direito, somem sozinhos).
// Qualquer tela chama notify(); o <Toaster /> do layout mostra a pilha.
export type ToastKind = 'success' | 'error' | 'info'
export type ToastItem = { id: number; text: string; kind: ToastKind; sha1?: string }

const MAX = 4
let nextId = 1
let items: ToastItem[] = []
const listeners = new Set<(items: ToastItem[]) => void>()
const recent = new Map<string, number>()

function emit() {
  listeners.forEach((listener) => listener(items))
}

export function notify(text: string, kind: ToastKind = 'success', options: { sha1?: string } = {}): void {
  const message = text.trim()
  if (!message) return
  // o mesmo balão duas vezes seguidas (efeito montado de novo, clique duplo) vira um só
  const key = `${kind}:${message}`
  const now = Date.now()
  if (now - (recent.get(key) ?? 0) < 400) return
  recent.set(key, now)
  items = [...items, { id: nextId++, text: message, kind, sha1: options.sha1 }].slice(-MAX)
  emit()
}

export function dismiss(id: number): void {
  items = items.filter((item) => item.id !== id)
  emit()
}

export function subscribeToasts(listener: (items: ToastItem[]) => void): () => void {
  listeners.add(listener)
  listener(items)
  return () => { listeners.delete(listener) }
}
