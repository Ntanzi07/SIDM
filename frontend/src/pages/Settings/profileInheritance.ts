import type { AnnotationProfile, BoxLabel, ObjectType, ProfileField } from '../../types/annotation'

// A herança no editor: o rascunho tem tudo (o herdado marcado com from, como
// o backend devolve). Ao trocar o pai, o que é do próprio perfil fica e o
// herdado vem do pai novo. O backend tira o que tem from ao salvar.

const mark = <T extends { from?: string }>(items: T[] | undefined, from: string): T[] =>
  (items ?? []).map((item) => ({ ...item, from: item.from ?? from }))

export const ownFields = (fields: ProfileField[]) => fields.filter((f) => !f.from)
export const ownLabels = (labels: BoxLabel[] | undefined) => (labels ?? []).filter((l) => !l.from)

// O que é do perfil num tipo de objeto: o tipo todo, ou (tipo do pai) só o acréscimo.
function ownPart(object: ObjectType): ObjectType | null {
  if (!object.from) return object
  const fields = ownFields(object.fields)
  const labels = ownLabels(object.box_labels)
  if (fields.length === 0 && labels.length === 0) return null
  return { key: object.key, label: object.label, fields, box_labels: labels }
}

// O índice do tipo como o backend conta ao conferir (só os que ficam ao gravar).
export function ownIndex(objects: ObjectType[], i: number): number {
  return objects.slice(0, i).filter((o) => ownPart(o) !== null).length
}

export function applyParent(draft: AnnotationProfile, parent: AnnotationProfile | undefined): AnnotationProfile {
  const mine = draft.objects.map(ownPart).filter((o): o is ObjectType => o !== null)
  if (!parent) {
    // sem pai: um acréscimo a um tipo herdado vira um tipo do próprio perfil
    return { ...draft, extends: undefined, lineage: undefined, fields: ownFields(draft.fields), objects: mine.map((o) => ({ ...o, from: undefined })) }
  }
  const objects: ObjectType[] = parent.objects.map((o) => ({
    ...o,
    from: o.from ?? parent.name,
    fields: mark(o.fields, parent.name),
    box_labels: mark(o.box_labels, parent.name),
  }))
  for (const object of mine) {
    const same = objects.find((o) => o.key === object.key)
    if (!same) {
      objects.push({ ...object, from: undefined })
      continue
    }
    same.fields = [...same.fields, ...ownFields(object.fields)]
    same.box_labels = [...(same.box_labels ?? []), ...ownLabels(object.box_labels)]
  }
  return {
    ...draft,
    extends: parent.name,
    lineage: [draft.name, ...(parent.lineage ?? [parent.name])],
    fields: [...mark(parent.fields, parent.name), ...ownFields(draft.fields)],
    objects,
  }
}

// Os perfis que dá para escolher como pai: nem o próprio, nem quem já o estende.
export function parentChoices(profiles: AnnotationProfile[], name: string): AnnotationProfile[] {
  if (!name) return profiles
  return profiles.filter((p) => p.name !== name && !(p.lineage ?? []).includes(name))
}
