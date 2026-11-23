// Quantas colunas uma grade tem (1 = lista). Fica guardado no navegador de
// quem usa, uma chave por tela.
export const COLUMNS = [5, 4, 3, 2, 1] as const
export type Columns = (typeof COLUMNS)[number]

export function savedColumns(key: string, fallback: Columns, allowed: readonly number[] = COLUMNS): Columns {
  try {
    const n = Number(localStorage.getItem(key))
    return allowed.includes(n) ? (n as Columns) : fallback
  } catch {
    return fallback
  }
}

export function saveColumns(key: string, value: Columns) {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // sem armazenamento (modo privado): vale só nesta visita
  }
}

export type MenuOption = { value: string; label: string }
