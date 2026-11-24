// O tema da tela: claro ou escuro. A escolha fica guardada neste navegador;
// sem escolha, vale o do sistema. O index.html aplica o mesmo antes do
// primeiro paint (a tela não pisca); aqui fica a troca pelo botão do header.
export type Theme = 'light' | 'dark'

const STORAGE_KEY = 'sidm.theme'
const listeners = new Set<(theme: Theme) => void>()

function initial(): Theme {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'light' || saved === 'dark') return saved
  } catch {
    // sem storage (modo privado): segue o sistema
  }
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

let current: Theme = initial()
document.documentElement.dataset.theme = current

export function getTheme(): Theme {
  return current
}

export function setTheme(theme: Theme): void {
  current = theme
  // troca de uma vez: sem as transições de cor, que deixavam campos e botões
  // no meio do caminho entre um tema e o outro
  const root = document.documentElement
  root.classList.add('theme-switching')
  root.dataset.theme = theme
  requestAnimationFrame(() => requestAnimationFrame(() => root.classList.remove('theme-switching')))
  try {
    localStorage.setItem(STORAGE_KEY, theme)
  } catch {
    // sem storage: vale só até recarregar a página
  }
  listeners.forEach((listener) => listener(theme))
}

export function subscribeTheme(listener: (theme: Theme) => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
