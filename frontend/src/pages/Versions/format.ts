import type { AnnotationProfile, ProfileField } from '../../types/annotation'
import { fieldLabel, optionLabel } from '../../utils/annotationForm'

// "23/11/2026 09:33"
export function formatWhen(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// O campo e o nome de uma chave de filtro/diff: "fields.cor",
// "objects.veiculo.cor" ou "objects.veiculo[2].cor" (o 2º veículo).
export function keyInfo(profile: AnnotationProfile | undefined, key: string): { label: string; field?: ProfileField } {
  if (key.startsWith('fields.')) {
    const name = key.slice('fields.'.length)
    const field = profile?.fields.find((item) => item.key === name)
    return { label: field ? fieldLabel(field) : name, field }
  }
  const match = /^objects\.([^.[]+)(?:\[(\d+)\])?\.(.+)$/.exec(key)
  if (match) {
    const [, typeKey, position, name] = match
    const type = profile?.objects.find((item) => item.key === typeKey)
    const field = type?.fields.find((item) => item.key === name)
    const typeLabel = type ? fieldLabel(type) : typeKey
    return { label: `${typeLabel}${position ? ` ${position}` : ''}: ${field ? fieldLabel(field) : name}`, field }
  }
  return { label: key }
}

// Um valor como a tela mostra; null = "Não informado".
export function valueText(field: ProfileField | undefined, value: unknown): string | null {
  if (value === null || value === undefined || value === '') return null
  if (Array.isArray(value)) {
    if (value.length === 0) return null
    return value.map((item) => valueText(field, item) ?? '').join(', ')
  }
  if (typeof value === 'boolean') return value ? 'Sim' : 'Não'
  if (typeof value === 'string') return field ? optionLabel(field, value) : value
  if (typeof value === 'number') return value.toLocaleString('pt-BR')
  return JSON.stringify(value)
}
