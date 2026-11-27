import type { BoxSet } from '../services/api'
import type { AnnotationProfile, ObjectType } from '../types/annotation'
import { taskColor } from './taskColors'

// Uma caixa: a tarefa e a posição dela na lista da tarefa.
export type BoxRef = { task: string; index: number }

// A cor de uma label: a escolhida na tarefa (Tarefas), ou uma fixa pelo nome
// (a mesma label tem sempre a mesma cor). Quem desenha chama useTaskColors()
// para redesenhar quando as cores escolhidas chegam.
export const BOX_COLORS = ['#e11d48', '#2563eb', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d']
export function colorOf(label: string, task?: string): string {
  const chosen = taskColor(task, label)
  if (chosen) return chosen
  return autoColor(label)
}

// O texto sobre a cor de uma label (a etiqueta da caixa): branco ou preto, o
// que tiver mais contraste (WCAG). A cor pode ser qualquer uma escolhida em
// Tarefas; com branco ou preto, uma das duas sempre passa de 4,5:1.
const INK_LIGHT = '#ffffff'
const INK_DARK = '#000000'

function luminance(hex: string): number | null {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) return null
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = parseInt(m[1].slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export function inkOn(background: string): string {
  const l = luminance(background)
  if (l === null) return INK_LIGHT
  const onWhite = 1.05 / (l + 0.05) // contraste do branco sobre a cor
  const onDark = (l + 0.05) / 0.05 // do preto sobre a cor
  return onWhite >= onDark ? INK_LIGHT : INK_DARK
}

export function autoColor(label: string): string {
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
