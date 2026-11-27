import { useState } from 'react'
import { FieldInput } from '../../pages/Annotation/AnnotationFields'
import { boxSetVersion, RequestError, updateBoxAttributes, type BoxSet, type Problem } from '../../services/api'
import type { AnnotationProfile, FormValues, ObjectType } from '../../types/annotation'
import { displayValue, fieldLabel, fromFormValue, toFormValue } from '../../utils/annotationForm'
import { boxPending, boxType, colorOf, type BoxRef } from '../../utils/boxes'
import { useTaskColors } from '../../utils/taskColors'
import { notify } from '../../services/toast'
import './BoxAttributes.css'

// O que foi digitado e ainda não salvo, por tarefa. Vale só para a versão
// das caixas em que foi digitado: depois de um reimport ou de salvar, some.
type Draft = { version: string; values: Record<number, FormValues> }

// Uma ação na caixa: virar um objeto da lista (ex.: "+ Veículo desta caixa").
export type BoxAction = { label: string; disabled?: boolean; run: () => void }

// As caixas do CVAT como objetos da imagem, por tarefa. Cada caixa é um card:
// clicar mostra na imagem; label com tipo de caixa no perfil mostra os campos
// (na selecionada); sem tipo, os atributos como vieram (ou nada). As caixas
// já ligadas a um objeto da lista (linked) aparecem nele, e aqui só o nome.
// readOnly: só mostra. collapsible: começa fechado, com um botão Exibir
// (abre sozinho quando uma caixa solta é escolhida na imagem).
export function BoxAttributes({ sets, profile, selected, onSelect, readOnly = false, onSaved, linked, actionFor, collapsible = false }: {
  sets: BoxSet[]
  profile: AnnotationProfile | null
  selected: BoxRef | null
  onSelect: (box: BoxRef | null) => void
  readOnly?: boolean
  onSaved?: () => void
  linked?: Map<string, string> // "tarefa#índice" → o objeto (ex.: "Veículo 1")
  actionFor?: (ref: BoxRef) => BoxAction | null
  collapsible?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [problems, setProblems] = useState<Record<string, Problem[]>>({})
  const [saving, setSaving] = useState('')
  useTaskColors()
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
    try {
      await updateBoxAttributes(set, attributes)
      setProblems((all) => ({ ...all, [set.task]: [] }))
      notify('Atributos salvos.')
      onSaved?.()
    } catch (error: unknown) {
      if (error instanceof RequestError && error.status === 409) {
        notify('As caixas mudaram (outra edição ou um novo import): recarregadas, confira e edite de novo.', 'error')
        onSaved?.()
      } else {
        setProblems((all) => ({ ...all, [set.task]: error instanceof RequestError ? error.problems : [] }))
        notify(error instanceof Error ? error.message : 'Não foi possível salvar os atributos.', 'error')
      }
    } finally {
      setSaving('')
    }
  }

  const free = withBoxes.reduce((n, set) => n + set.boxes.filter((_, i) => !linked?.has(`${set.task}#${i}`)).length, 0)
  const freePending = withBoxes.reduce((n, set) => n + set.boxes.filter((_, i) => !linked?.has(`${set.task}#${i}`) && pending({ task: set.task, index: i })).length, 0)
  // uma caixa solta escolhida (na imagem ou pelo atalho) abre a lista: os campos dela ficam aqui
  const selectedFree = selected !== null && withBoxes.some((set) => set.task === selected.task) && !linked?.has(`${selected.task}#${selected.index}`)
  const shown = !collapsible || open || selectedFree
  const title = linked ? 'Outros objetos da imagem' : 'Caixas'

  return (
    <div className={shown ? 'box-attributes' : 'box-attributes box-attributes--closed'}>
      {collapsible ? (
        <div className="box-attributes__top">
          <h3>
            {title} <span className="box-attributes__count">{free}</span>
            {freePending > 0 && <span className="box-attributes__pending" title="atributo obrigatório faltando">!</span>}
          </h3>
          <button
            aria-expanded={shown}
            className="button button--secondary button--sm"
            onClick={() => {
              if (!shown) { setOpen(true); return }
              setOpen(false)
              if (selectedFree) onSelect(null) // fechou: a caixa solta deixa de estar escolhida
            }}
            type="button"
          >
            {shown ? 'Ocultar' : 'Exibir'}
          </button>
        </div>
      ) : (
        <h3>{linked ? `${title} (caixas)` : title}</h3>
      )}
      {shown && <>
        {linked && free === 0 && <p className="box-attributes__hint">Todas as caixas já são objetos da lista acima.</p>}
        {withBoxes.map((set) => {
          const dirty = Object.keys(draftOf(set)).length > 0
          const pick = selected?.task === set.task ? selected.index : -1
          const problemAt = (index: number, key: string) => problems[set.task]?.find((p) => p.path === `boxes[${index}].attributes.${key}`)?.msg
          const joined = set.boxes.flatMap((b, index) => {
            const name = linked?.get(`${set.task}#${index}`)
            return name ? [{ index, label: b.label, name }] : []
          })
          return (
            <div className="box-attributes__task" key={set.task}>
              <div className="box-attributes__heading">
                <strong>{set.task}</strong>
                {set.updated_by && <small>atributos editados por {set.updated_by}</small>}
              </div>
              <div className="box-objects">
                {set.boxes.map((b, index) => {
                  if (linked?.has(`${set.task}#${index}`)) return null
                  const ref = { task: set.task, index }
                  const type = boxType(profile, b.label)
                  const on = index === pick
                  const attrs = Object.entries(b.attributes ?? {})
                  const action = actionFor?.(ref) ?? null
                  return (
                    <div className={on ? 'box-object box-object--on' : 'box-object'} key={index} style={{ borderLeftColor: colorOf(b.label, set.task) }}>
                      <div className="box-object__head">
                        <button className="box-object__name" onClick={() => onSelect(on ? null : ref)} title="Ver na imagem" type="button">
                          {index + 1}. {b.label}{pending(ref) && <span className="box-attributes__pending" title="atributo obrigatório faltando">!</span>}
                        </button>
                        {action && (
                          <button className="button button--secondary button--sm" disabled={action.disabled} onClick={action.run} type="button">{action.label}</button>
                        )}
                      </div>
                      {type && !readOnly && on && (
                        <div className="box-attributes__fields">
                          {type.fields.length === 0 && <p className="box-attributes__hint">O tipo {fieldLabel(type)} não tem campos.</p>}
                          {type.fields.map((field) => (
                            <FieldInput
                              error={problemAt(index, field.key)}
                              field={field}
                              key={field.key}
                              onChange={(value) => change(set, index, type, field.key, value)}
                              value={valuesOf(set, index, type)[field.key] ?? ''}
                            />
                          ))}
                        </div>
                      )}
                      {type && (readOnly || !on) && type.fields.length > 0 && (
                        <dl className="box-attributes__values">
                          {type.fields.map((field) => (
                            <div key={field.key}>
                              <dt>{fieldLabel(field)}</dt>
                              <dd>{displayValue(field, (readOnly ? undefined : draftOf(set)[index]?.[field.key]) ?? b.attributes?.[field.key])}</dd>
                            </div>
                          ))}
                        </dl>
                      )}
                      {!type && attrs.length > 0 && (
                        <dl className="box-attributes__values">
                          {attrs.map(([key, value]) => (
                            <div key={key}><dt>{key}</dt><dd>{Array.isArray(value) ? value.join(', ') : String(value)}</dd></div>
                          ))}
                        </dl>
                      )}
                    </div>
                  )
                })}
              </div>
              {joined.length > 0 && (
                <p className="box-attributes__hint">
                  Já são objetos da lista:{' '}
                  {joined.map((j, k) => (
                    <span key={j.index}>
                      {k > 0 && ', '}
                      <button className="box-object__link" onClick={() => onSelect({ task: set.task, index: j.index })} type="button">{j.index + 1}. {j.label}</button> ({j.name})
                    </span>
                  ))}
                </p>
              )}

              {!readOnly && dirty && (
                <div className="box-attributes__actions">
                  <button className="button button--primary button--sm" disabled={saving !== ''} onClick={() => void save(set)} type="button">
                    {saving === set.task ? 'Salvando...' : 'Salvar atributos'}
                  </button>
                </div>
              )}
            </div>
          )
        })}
      </>}
    </div>
  )
}
