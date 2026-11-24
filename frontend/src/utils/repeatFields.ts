import type { FormValue } from '../types/annotation'

// Campos com "repeat" no perfil de anotação (no veiculos: equipamento,
// câmera, direção, ângulo e clima): no mesmo pacote, a próxima imagem
// pendente já vem com o valor da última que a pessoa salvou. Os valores
// ficam no navegador, por pacote (sidm:last:<pacote>).

const lastKey = (pkg: string) => `sidm:last:${pkg}`

export function loadLast(pkg: string): Record<string, FormValue> {
  try {
    const raw = localStorage.getItem(lastKey(pkg))
    return raw ? (JSON.parse(raw) as Record<string, FormValue>) : {}
  } catch {
    return {} // navegador sem localStorage ou JSON estragado: só não repete
  }
}

export function saveLast(pkg: string, values: Record<string, FormValue>) {
  try {
    localStorage.setItem(lastKey(pkg), JSON.stringify(values))
  } catch {
    // sem espaço ou sem permissão: o recurso é só uma comodidade
  }
}

// Vazio: "", lista vazia ou ausente.
export const isEmptyValue = (value: unknown) =>
  value === undefined || value === null || value === '' || (Array.isArray(value) && value.length === 0)
