import { useState } from 'react'
import { FieldInput } from '../../pages/Annotation/AnnotationFields'
import { boxSetVersion, RequestError, updateBoxAttributes, type BoxSet, type Problem } from '../../services/api'
import type { AnnotationProfile, FormValues, ObjectType } from '../../types/annotation'
import { displayValue, fieldLabel, fromFormValue, toFormValue } from '../../utils/annotationForm'
import { boxPending, boxType, colorOf, type BoxRef } from '../../utils/boxes'
import './BoxAttributes.css'

// O que foi digitado e ainda não salvo, por tarefa. Vale só para a versão
// das caixas em que foi digitado: depois de um reimport ou de salvar, some.
type Draft = { version: string; values: Record<number, FormValues> }

// Os atributos das caixas do CVAT, por tarefa. A caixa selecionada (na imagem
// ou na lista) mostra os campos do tipo de objeto do perfil; label sem tipo
// mostra os atributos como vieram, só leitura. readOnly: só mostra.
export function BoxAttributes({ sets, profile, selected, onSelect, readOnly = false, onSaved }: {
  sets: BoxSet[]
  profile: AnnotationProfile | null
  selected: BoxRef | null
  onSelect: (box: BoxRef | null) => void
  readOnly?: boolean
  onSaved?: () => void
}) {
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [problems, setProblems] = useState<Record<string, Problem[]>>({})
  const [message, setMessage] = useState<{ task: string; text: string; error: boolean } | null>(null)
  const [saving, setSaving] = useState('')
  const pending = boxPending(profile, sets)
  const withBoxes = sets.filter((set) => set.boxes.length > 0)
  if (withBoxes.length === 0) return null

  const draftOf = (set: BoxSet) => {
    const draft = drafts[set.task]
    return draft && draft.version === boxSetVersion(set) ? draft.values : {}
  }

  // Os valores (no formato do formulário) de uma caixa: o digitado ou o guardado.
  function valuesOf(set: BoxSet, index: number, type: ObjectType): FormValues {
    const typed = draftOf(set)[index]
    if (typed) return typed
    const stored = set.boxes[index].attributes ?? {}
    return Object.fromEntries(type.fields.map((field) => [field.key, toFormValue(field, stored[field.key])]))
  }

  function change(set: BoxSet, index: number, type: ObjectType, key: string, value: FormValues[string]) {
    const current = valuesOf(set, index, type)
    setDrafts((all) => ({
      ...all,
      [set.task]: { version: boxSetVersion(set), values: { ...draftOf(set), [index]: { ...current, [key]: value } } },
    }))
    setMessage(null)
  }

  async function save(set: BoxSet) {
    const attributes = set.boxes.map((box, index) => {
      const type = boxType(profile, box.label)
      if (!type) return box.attributes ?? {} // sem tipo: o backend mantém como está
      const values = valuesOf(set, index, type)
      const out: Record<string, unknown> = {}
      for (const field of type.fields) {
        const value = fromFormValue(field, values[field.key] ?? '')
        if (value !== undefined) out[field.key] = value
      }
      return out
    })
    setSaving(set.task)
    setMessage(null)
    try {
      await updateBoxAttributes(set, attributes)
      setProblems((all) => ({ ...all, [set.task]: [] }))
      setMessage({ task: set.task, text: 'Atributos salvos.', error: false })
      onSaved?.()
    } catch (error: unknown) {
      if (error instanceof RequestError && error.status === 409) {
        setMessage({ task: set.task, text: 'As caixas mudaram (outra edição ou um novo import do CVAT): recarregadas, confira e edite de novo.', error: true })
        onSaved?.()
      } else {
        setProblems((all) => ({ ...all, [set.task]: error instanceof RequestError ? error.problems : [] }))
        setMessage({ task: set.task, text: error instanceof Error ? error.message : 'Não foi possível salvar os atributos.', error: true })
      }
    } finally {
      setSaving('')
    }
  }

  return (
    <div className="box-attributes">
      <h3>Caixas do CVAT</h3>
      {!selected && <p className="box-attributes__hint">Clique numa caixa (na imagem ou aqui) para ver os atributos.</p>}
      {withBoxes.map((set) => {
        const dirty = Object.keys(draftOf(set)).length > 0
        const pick = selected?.task === set.task ? selected.index : -1
        const box = pick >= 0 ? set.boxes[pick] : undefined
        const type = box ? boxType(profile, box.label) : undefined
        const problemAt = (key: string) => problems[set.task]?.find((p) => p.path === `boxes[${pick}].attributes.${key}`)?.msg
        return (
          <div className="box-attributes__task" key={set.task}>
            <div className="box-attributes__heading">
              <strong>{set.task}</strong>
              {set.updated_by && <small>atributos editados por {set.updated_by}</small>}
            </div>
            <div className="box-attributes__list">
              {set.boxes.map((b, index) => {
                const ref = { task: set.task, index }
                return (
                  <button
                    className={index === pick ? 'box-attributes__chip box-attributes__chip--on' : 'box-attributes__chip'}
                    key={index}
                    onClick={() => onSelect(index === pick ? null : ref)}
                    style={{ borderColor: colorOf(b.label) }}
                    title={pending(ref) ? 'atributo obrigatório faltando' : undefined}
                    type="button"
                  >
                    {index + 1}. {b.label}{pending(ref) && <span className="box-attributes__pending">!</span>}
                  </button>
                )
              })}
            </div>

            {box && type && !readOnly && (
              <div className="box-attributes__fields">
                {type.fields.length === 0 && <p className="box-attributes__hint">O tipo {fieldLabel(type)} não tem campos.</p>}
                {type.fields.map((field) => (
                  <FieldInput
                    error={problemAt(field.key)}
                    field={field}
                    key={field.key}
                    onChange={(value) => change(set, pick, type, field.key, value)}
                    value={valuesOf(set, pick, type)[field.key] ?? ''}
                  />
                ))}
              </div>
            )}
            {box && type && readOnly && (
              <dl className="box-attributes__values">
                {type.fields.map((field) => (
                  <div key={field.key}>
                    <dt>{fieldLabel(field)}</dt>
                    <dd>{displayValue(field, box.attributes?.[field.key])}</dd>
                  </div>
                ))}
              </dl>
            )}
            {box && !type && (
              <>
                <p className="box-attributes__hint">
                  A label "{box.label}" não é um tipo de objeto com caixa do perfil: os atributos ficam como vieram do CVAT.
                </p>
                <dl className="box-attributes__values">
                  {Object.entries(box.attributes ?? {}).map(([key, value]) => (
                    <div key={key}><dt>{key}</dt><dd>{Array.isArray(value) ? value.join(', ') : String(value)}</dd></div>
                  ))}
                  {Object.keys(box.attributes ?? {}).length === 0 && <div><dt>Atributos</dt><dd>nenhum</dd></div>}
                </dl>
              </>
            )}

            {!readOnly && (dirty || message?.task === set.task) && (
              <div className="box-attributes__actions">
                {message?.task === set.task && (
                  <span className={message.error ? 'form-error' : 'settings-message'}>{message.text}</span>
                )}
                {dirty && (
                  <button className="button button--primary button--sm" disabled={saving !== ''} onClick={() => void save(set)} type="button">
                    {saving === set.task ? 'Salvando...' : 'Salvar atributos'}
                  </button>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
