// As cores das categorias nos gráficos (tons que funcionam no fundo escuro).
export const PALETTE = ['#ff8a4c', '#6fd6a2', '#7aa7ff', '#f0d264', '#c792ea', '#ff6b8b', '#5fd0d6', '#b5c46a', '#e8a86b', '#9aa0ff']
export const MISSING_COLOR = '#45403a'

export const fmt = (n: number) => n.toLocaleString('pt-BR')
export const pctText = (n: number, of: number) => (of > 0 ? `${Math.round((n / of) * 100)}%` : '0%')

const DAY = 86400000

// "3 dias", "5 h", "12 min"
export function spanText(ms: number): string {
  if (ms >= DAY) {
    const days = Math.round(ms / DAY)
    return `${days} dia${days === 1 ? '' : 's'}`
  }
  if (ms >= 3600000) return `${Math.round(ms / 3600000)} h`
  return `${Math.max(1, Math.round(ms / 60000))} min`
}

