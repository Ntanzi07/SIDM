import type { AnnotationProfile, ProfileField } from '../types/annotation'
import { fieldLabel, optionLabel } from './annotationForm'

// O que a Pesquisa e as Versões usam para mostrar contagens.

export const fmt = (n: number) => n.toLocaleString('pt-BR')
export const pct = (n: number, of: number) => (of > 0 ? `${Math.round((n / of) * 100)}%` : '0%')

export const toggle = (list: string[], item: string) => (list.includes(item) ? list.filter((x) => x !== item) : [...list, item])

// A pesquisa conta por valor só os campos de lista e sim/não (como o backend).
export const countable = (field: ProfileField) => field.type === 'choice' || field.type === 'multi' || field.type === 'bool'

// Um campo que a tela mostra: da imagem ("fields.<campo>") ou de um tipo de
// objeto sem caixa ("objects.<tipo>.<campo>").
export type Facet = {
  key: string // a chave do filtro na query
  title: string
  field: ProfileField
  objectType?: string
}

export function facetsOf(profile: AnnotationProfile): Facet[] {
  const out: Facet[] = []
  for (const field of profile.fields.filter(countable)) {
    out.push({ key: `fields.${field.key}`, title: fieldLabel(field), field })
  }
  for (const type of profile.objects.filter((item) => !item.box)) {
    for (const field of type.fields.filter(countable)) {
      out.push({ key: `objects.${type.key}.${field.key}`, title: `${fieldLabel(type)}: ${fieldLabel(field)}`, field, objectType: type.key })
    }
  }
  return out
}

// Os valores possíveis, na ordem do perfil (os que não aparecem contam zero).
export function valuesOf(field: ProfileField): Array<{ value: string; label: string }> {
  if (field.type === 'bool') return [{ value: 'true', label: 'Sim' }, { value: 'false', label: 'Não' }]
  return (field.options ?? []).map((option) => ({ value: option.value, label: optionLabel(field, option.value) }))
}
