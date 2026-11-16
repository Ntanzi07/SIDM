// O <input type="datetime-local"> trabalha com a hora LOCAL e sem fuso
// ("2026-08-12T09:23:23"); o backend guarda e devolve em UTC ISO
// ("2026-08-12T12:23:23Z"). Estas funções fazem a conversão nos dois
// sentidos. Cortar a string ISO (slice) mostraria a hora UTC como se fosse
// local: 3 horas de diferença no Brasil.

const pad = (n: number) => String(n).padStart(2, '0')

export function isoToLocalInput(iso: string | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
    + `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

// "2026-08-12T09:23:23" (hora local) -> "2026-08-12T12:23:23.000Z"
export function localInputToISO(value: string): string | null {
  if (!value) return null
  const date = new Date(value) // sem fuso na string: o navegador lê como hora local
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

// Diurno das 06:00 às 17:59, pela hora local do input.
export function guessDayTime(localInput: string): 'day' | 'night' | '' {
  const hour = Number.parseInt(localInput.slice(11, 13), 10)
  if (Number.isNaN(hour)) return ''
  return hour >= 6 && hour < 18 ? 'day' : 'night'
}
