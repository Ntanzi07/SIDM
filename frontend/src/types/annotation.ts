// Os tipos do perfil de anotação (espelham o módulo annotation do backend):
// o que se anota em cada imagem de um pacote.

export type FieldType = 'choice' | 'multi' | 'text' | 'number' | 'bool' | 'datetime' | 'ref'

export type ProfileOption = {
  value: string
  label?: string
  code?: string // a forma curta no nome do arquivo
}

export type ProfileField = {
  key: string
  label?: string
  type: FieldType
  required?: boolean
  help?: string
  default?: unknown // o valor que a tela já traz numa imagem sem rascunho
  options?: ProfileOption[] // choice, multi
  pattern?: string // text: o valor inteiro
  max_len?: number
  upper?: boolean
  min?: number // number
  max?: number
  integer?: boolean
  ref?: 'equipment' | 'camera'
}

export type ObjectType = {
  key: string
  label?: string
  box?: boolean // vem das caixas do CVAT (não é anotado aqui)
  min?: number
  max?: number // 0 = sem limite
  fields: ProfileField[]
}

export type AnnotationProfile = {
  name: string
  description?: string
  builtin?: boolean
  fields: ProfileField[]
  objects: ObjectType[]
  created_by?: string
  updated_at?: string
}

// Um objeto anotado (sem caixa), como a API guarda.
export type AnnotationObject = {
  type: string
  fields: Record<string, unknown>
}

export type ImageDetail = {
  sha1: string
  package: string
  status: 'pending' | 'verified'
  original_name: string
  file_name?: string
  pattern_version?: string
  ext: string
  size: number
  fields: Record<string, unknown>
  objects: AnnotationObject[]
  claimed_by?: string
  claimed_until?: string
  updated_at?: string
}

// O formulário guarda tudo como texto (o que os <input> e <select> usam):
// número e sim/não também; a data no formato do datetime-local; multi como
// lista. "" é "não informado". A conversão para a API fica em
// utils/annotationForm.ts.
export type FormValue = string | string[]
export type FormValues = Record<string, FormValue>
export type FormObject = { type: string; values: FormValues }
export type AnnotationForm = { fields: FormValues; objects: FormObject[] }

export type AnnotationCodes = {
  direction?: Record<string, string>
  resolution?: Record<string, string>
  capture_angle?: Record<string, string>
  day_time?: Record<string, string>
  image_range?: Record<string, string>
  weather?: Record<string, string>
  conditions?: Record<string, string>
  plate_type?: Record<string, string>
  plate_category?: Record<string, string>
  vehicle_type?: Record<string, string>
  vehicle_color?: Record<string, string>
}
