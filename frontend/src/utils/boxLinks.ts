import type { Box, BoxSet } from '../services/api'
import type { FormObject, ObjectBox, ObjectType } from '../types/annotation'
import type { BoxRef } from './boxes'

// A ligação objeto ↔ caixa na tela. As caixas não têm id: a ligação guarda
// a tarefa, a label e a posição, e aqui se acha a caixa de novo (a mesma
// posição, ou a mais parecida se a tarefa foi importada de novo e ela
// mexeu um pouco).

export const toObjectBox = (task: string, box: Box): ObjectBox => ({ task, label: box.label, x: box.x, y: box.y, w: box.w, h: box.h })

// Quanto duas caixas se cobrem (0 a 1).
function overlap(a: ObjectBox, b: Box): number {
  const ix = Math.max(0, Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x))
  const iy = Math.max(0, Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y))
  const inter = ix * iy
  const union = a.w * a.h + b.w * b.h - inter
  return union > 0 ? inter / union : 0
}

// A caixa de uma ligação nas caixas de hoje (null: sumiu).
export function findLinked(sets: BoxSet[], ref: ObjectBox): BoxRef | null {
  const set = sets.find((s) => s.task === ref.task)
  if (!set) return null
  let best: BoxRef | null = null
  let score = 0.5 // menos que isso não é a mesma caixa
  set.boxes.forEach((box, index) => {
    if (box.label !== ref.label) return
    const o = overlap(ref, box)
    if (o > score || (o === 1 && score === 1)) {
      best = { task: set.task, index }
      score = o
    }
  })
  return best
}

// As caixas da imagem que podem ser um objeto do tipo (label nas box_labels).
export type Candidate = { ref: BoxRef; box: Box; key: string; name: string }
export function candidates(type: ObjectType, sets: BoxSet[]): Candidate[] {
  const labels = new Set((type.box_labels ?? []).map((label) => label.label))
  const out: Candidate[] = []
  for (const set of sets) {
    set.boxes.forEach((box, index) => {
      if (labels.has(box.label)) {
        out.push({ ref: { task: set.task, index }, box, key: `${set.task}#${index}`, name: `${set.task} · ${index + 1}. ${box.label}` })
      }
    })
  }
  return out
}

// Qual caixa cada objeto do formulário aponta (índice do objeto → chave da caixa).
export function linkedKeys(objects: FormObject[], sets: BoxSet[]): Map<number, string> {
  const out = new Map<number, string>()
  objects.forEach((object, i) => {
    if (!object.box) return
    const ref = findLinked(sets, object.box)
    if (ref) out.set(i, `${ref.task}#${ref.index}`)
  })
  return out
}
