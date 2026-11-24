import type {
  AnnotationForm,
  AnnotationObject,
  AnnotationProfile,
  FormObject,
  FormValue,
  FormValues,
  ImageDetail,
  ObjectBox,
  ObjectType,
  ProfileField,
} from '../types/annotation'
import { guessDayTime, isoToLocalInput, localInputToISO } from './datetime'

// Conversão entre o que a API guarda (string, número, true/false, lista,
// data ISO) e o que o formulário mostra (sempre texto, ou lista no multi).

export const fieldLabel = (field: { key: string; label?: string }) => field.label || field.key

export function optionLabel(field: ProfileField, value: string): string {
  return field.options?.find((option) => option.value === value)?.label || value
}

// API -> formulário
export function toFormValue(field: ProfileField, value: unknown): FormValue {
  if (value === undefined || value === null) return field.type === 'multi' ? [] : ''
  switch (field.type) {
    case 'multi':
      return Array.isArray(value) ? value.map(String) : [String(value)]
    case 'datetime':
      return isoToLocalInput(String(value))
    case 'bool':
      return value === true ? 'true' : value === false ? 'false' : ''
    default:
      return String(value)
  }
}

// O valor inicial de um campo numa imagem sem rascunho.
export const initialValue = (field: ProfileField): FormValue => toFormValue(field, field.default)

// Formulário -> API. Vazio some (é o mesmo que não informado).
export function fromFormValue(field: ProfileField, value: FormValue): unknown {
  if (Array.isArray(value)) return value.length ? value : undefined
  const text = value.trim()
  if (text === '') return undefined
  switch (field.type) {
    case 'number':
      return Number(text.replace(',', '.'))
    case 'bool':
      return text === 'true'
    case 'datetime':
      return localInputToISO(text) ?? text // inválida: o backend explica o erro
    case 'text':
      return field.upper ? text.toUpperCase() : text
    default:
      return text
  }
}

function valuesFrom(fields: ProfileField[], stored: Record<string, unknown> | undefined, useDefaults: boolean): FormValues {
  const out: FormValues = {}
  for (const field of fields) {
    const has = stored && stored[field.key] !== undefined && stored[field.key] !== null
    out[field.key] = has ? toFormValue(field, stored[field.key]) : useDefaults ? initialValue(field) : toFormValue(field, undefined)
  }
  return out
}

// Os tipos de objeto anotados aqui (os com caixa vêm do CVAT).
export const listObjectTypes = (profile: AnnotationProfile) => profile.objects.filter((type) => !type.box)

export function newObject(type: ObjectType, box?: ObjectBox): FormObject {
  const object: FormObject = { type: type.key, values: valuesFrom(type.fields, undefined, true) }
  return box ? withBox(type, object, box) : object
}

// Liga (ou desliga, com box undefined) a caixa de um objeto. O que a label
// da caixa diz vale para o objeto (motorcycle → tipo moto): troca o valor,
// porque a caixa manda (o backend recusaria um valor diferente).
export function withBox(type: ObjectType, object: FormObject, box: ObjectBox | undefined): FormObject {
  if (!box) return { type: object.type, values: object.values }
  const implied = type.box_labels?.find((label) => label.label === box.label)?.fields ?? {}
  const values = { ...object.values }
  for (const [key, value] of Object.entries(implied)) {
    const field = type.fields.find((f) => f.key === key)
    if (field) values[key] = toFormValue(field, value)
  }
  return { type: object.type, values, box }
}

// Os campos que a caixa ligada já diz (a tela trava esses).
export function impliedKeys(type: ObjectType, object: FormObject): string[] {
  if (!object.box) return []
  return Object.keys(type.box_labels?.find((label) => label.label === object.box?.label)?.fields ?? {})
}

// Placa Mercosul: AAA1A11 (letra na 5ª posição). Antiga: AAA1111. Só
// preenche quando o tipo da placa veio vazio (ex.: import sem essa coluna).
function guessPlateType(values: FormValues) {
  if (!('plate' in values) || !('plate_type' in values) || values.plate_type) return
  const plate = String(values.plate).replace(/[\s-]/g, '').toUpperCase()
  if (/^[A-Z]{3}\d[A-Z]\d{2}$/.test(plate)) values.plate_type = 'mercosul'
  else if (/^[A-Z]{3}\d{4}$/.test(plate)) values.plate_type = 'br'
}

// O formulário de uma imagem. Sem rascunho nenhum, os campos vêm com o valor
// inicial do perfil e já há um objeto de cada tipo (o que quase toda imagem
// tem: dá para remover).
export function formFromImage(profile: AnnotationProfile, image: ImageDetail): AnnotationForm {
  const stored = image.fields ?? {}
  const fresh = Object.keys(stored).length === 0 && (image.objects ?? []).length === 0
  const fields = valuesFrom(profile.fields, stored, true)

  // Período pela hora da captura, quando o rascunho não diz.
  if ('day_time' in fields && stored.day_time === undefined && typeof fields.captured_at === 'string') {
    fields.day_time = guessDayTime(fields.captured_at) || fields.day_time
  }

  const types = listObjectTypes(profile)
  let objects: FormObject[]
  if (fresh) {
    objects = types.flatMap((type) => Array.from({ length: Math.max(type.min ?? 0, 1) }, () => newObject(type)))
  } else {
    objects = (image.objects ?? []).flatMap((object: AnnotationObject) => {
      const type = types.find((t) => t.key === object.type)
      if (!type) return [] // tipo que o perfil não tem mais: o backend recusaria
      return [{ type: type.key, values: valuesFrom(type.fields, object.fields, false), box: object.box }]
    })
  }
  objects.forEach((object) => guessPlateType(object.values))
  return { fields, objects }
}

// O corpo do PUT /images/{sha1}.
export function formToAPI(profile: AnnotationProfile, form: AnnotationForm) {
  const pack = (defs: ProfileField[], values: FormValues) => {
    const out: Record<string, unknown> = {}
    for (const field of defs) {
      const value = fromFormValue(field, values[field.key] ?? '')
      if (value !== undefined) out[field.key] = value
    }
    return out
  }
  return {
    fields: pack(profile.fields, form.fields),
    objects: form.objects.map((object) => {
      const type = profile.objects.find((t) => t.key === object.type)
      return { type: object.type, fields: type ? pack(type.fields, object.values) : {}, ...(object.box ? { box: object.box } : {}) }
    }),
  }
}

// Como mostrar um valor guardado (visualizador).
export function displayValue(field: ProfileField, value: unknown): string {
  if (value === undefined || value === null || value === '' || (Array.isArray(value) && value.length === 0)) return 'Não informado'
  switch (field.type) {
    case 'choice':
      return optionLabel(field, String(value))
    case 'multi':
      return (Array.isArray(value) ? value : [value]).map((v) => optionLabel(field, String(v))).join(', ')
    case 'bool':
      return value === true ? 'Sim' : 'Não'
    case 'datetime': {
      const date = new Date(String(value))
      return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('pt-BR')
    }
    default:
      return String(value)
  }
}
