// Confirmações do site num modal (no lugar do window.confirm do navegador).
// Uso: if (!(await confirmDialog({ title, message }))) return
// O <ConfirmDialog /> do layout mostra uma de cada vez, na ordem.
export type ConfirmOptions = {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: 'primary' | 'danger' // danger: a ação apaga ou não se desfaz
}

export type ConfirmRequest = ConfirmOptions & { id: number; resolve: (ok: boolean) => void }

let nextId = 1
let queue: ConfirmRequest[] = []
const listeners = new Set<(current: ConfirmRequest | null) => void>()

function emit() {
  listeners.forEach((listener) => listener(queue[0] ?? null))
}

export function confirmDialog(options: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    queue = [...queue, { ...options, id: nextId++, resolve }]
    emit()
  })
}

export function answerConfirm(id: number, ok: boolean): void {
  const request = queue.find((item) => item.id === id)
  if (!request) return
  queue = queue.filter((item) => item.id !== id)
  request.resolve(ok)
  emit()
}

export function subscribeConfirm(listener: (current: ConfirmRequest | null) => void): () => void {
  listeners.add(listener)
  listener(queue[0] ?? null)
  return () => { listeners.delete(listener) }
}
