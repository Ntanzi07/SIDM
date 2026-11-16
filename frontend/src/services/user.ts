// Quem está usando o sistema. Ainda não há login: o nome vai no header
// X-User de cada pedido (histórico das imagens e reserva da fila) e fica
// guardado neste navegador.
const STORAGE_KEY = 'sidm.user'

let current = read()
const listeners = new Set<(user: string) => void>()

function read(): string {
  try {
    return localStorage.getItem(STORAGE_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

export function getUser(): string {
  return current
}

export function setUser(name: string): void {
  current = name.trim()
  try {
    if (current) localStorage.setItem(STORAGE_KEY, current)
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // sem storage (modo privado etc.): vale só até recarregar a página
  }
  listeners.forEach((listener) => listener(current))
}

export function subscribeUser(listener: (user: string) => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
