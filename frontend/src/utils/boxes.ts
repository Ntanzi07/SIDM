import type { BoxSet } from '../services/api'
import type { AnnotationProfile, ObjectType } from '../types/annotation'

// Uma caixa: a tarefa e a posição dela na lista da tarefa.
export type BoxRef = { task: string; index: number }

// Cor fixa por label (a mesma label tem sempre a mesma cor).
const BOX_COLORS = ['#e11d48', '#2563eb', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d']
export function colorOf(label: string): string {
  let hash = 0
  for (const char of label) hash = (hash * 31 + char.charCodeAt(0)) | 0
  return BOX_COLORS[Math.abs(hash) % BOX_COLORS.length]
}

// O tipo de objeto com caixa de uma label (a label do CVAT é a chave do tipo).
export function boxType(profile: AnnotationProfile | null, label: string): ObjectType | undefined {
  return profile?.objects.find((type) => type.box && type.key === label)
}

const isEmpty = (value: unknown) =>
  value === undefined || value === null || value === '' || (Array.isArray(value) && value.length === 0)

// A caixa tem atributo obrigatório vazio? (não impede de verificar a imagem:
// só aparece como pendência)
export function boxPending(profile: AnnotationProfile | null, sets: BoxSet[]) {
  return ({ task, index }: BoxRef) => {
    const box = sets.find((set) => set.task === task)?.boxes[index]
    const type = box && boxType(profile, box.label)
    return Boolean(type?.fields.some((field) => field.required && isEmpty(box?.attributes?.[field.key])))
  }
}
