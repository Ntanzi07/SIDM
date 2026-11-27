import type { Problem } from '../../services/api'
import type { FieldType, ProfileField } from '../../types/annotation'

// As contas do editor de perfil de anotação (ProfileEditor.tsx): nomes,
// chaves, tipos e o caminho dos problemas que o backend devolve.

export const TYPES: Array<[FieldType, string]> = [
  ['choice', 'Lista (um valor)'],
  ['multi', 'Lista (vários valores)'],
  ['text', 'Texto'],
  ['number', 'Número'],
  ['bool', 'Sim/não'],
  ['datetime', 'Data e hora'],
  ['ref', 'Cadastro (equipamento/câmera)'],
]
export const typeName = (type: FieldType) => TYPES.find(([t]) => t === type)?.[1] ?? type

// "Cor do veículo" -> "cor_do_veiculo" (o formato que o backend aceita).
export function slug(text: string): string {
  const s = text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 32)
  return /^[a-z]/.test(s) ? s : s ? `c_${s}`.slice(0, 32) : ''
}

// "Preto fosco" -> "preto-fosco" (valor de opção).
export const optionValue = (text: string) => slug(text).replace(/_/g, '-').replace(/^c-/, '') || ''

// O nome do perfil vai na URL: minúsculas, números e hífen (enquanto digita).
export const profileName = (text: string) =>
  text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+/, '').slice(0, 48)

export const newField = (): ProfileField => ({ key: '', label: '', type: 'choice', options: [] })

// Ao trocar o tipo, some o que só valia para o tipo antigo.
export function withType(field: ProfileField, type: FieldType): ProfileField {
  const base: ProfileField = { key: field.key, label: field.label, type, required: field.required, help: field.help, repeat: field.repeat }
  // "vem da imagem" só em lista ou texto
  if (field.from_image && (type === 'choice' || type === 'text')) base.from_image = field.from_image
  if (type === 'choice' || type === 'multi') base.options = field.options ?? []
  if (type === 'ref') {
    base.ref = 'equipment'
    base.key = 'equipment' // o backend exige: a cascata de renomear procura por esse nome
  }
  return base
}

// Problemas de um caminho e de tudo abaixo dele.
export const under = (problems: Problem[], path: string) =>
  problems.filter((p) => p.path === path || p.path.startsWith(`${path}.`) || p.path.startsWith(`${path}[`))

// O id do elemento de um caminho ("objects[0].fields[1]" -> "pe-objects-0-fields-1").
export const pathId = (path: string) => `pe-${path.replace(/[^a-z0-9]+/gi, '-').replace(/-$/, '')}`

// O resumo de um campo na linha fechada.
export function summary(field: ProfileField): string {
  const parts = [typeName(field.type)]
  if (field.type === 'choice' || field.type === 'multi') {
    const n = field.options?.length ?? 0
    parts.push(n === 1 ? '1 opção' : `${n} opções`)
  }
  if (field.type === 'ref') parts.push(field.ref === 'camera' ? 'câmeras' : 'equipamentos')
  if (field.required) parts.push('obrigatório')
  if (field.repeat) parts.push('repete a última')
  if (field.from_image) parts.push('vem da imagem')
  return parts.join(', ')
}

