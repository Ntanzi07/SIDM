import type { BoxSet } from '../../services/api'
import type { FormObject, ObjectBox, ObjectType } from '../../types/annotation'
import type { BoxRef } from '../../utils/boxes'
import { candidates, findLinked, toObjectBox } from '../../utils/boxLinks'
import { boxAttributeNotes, displayValue, fieldLabel, optionLabel } from '../../utils/annotationForm'

type Props = {
  type: ObjectType
  object: FormObject
  sets: BoxSet[]
  taken: Set<string> // as caixas que outros objetos já apontam
  onLink: (box: ObjectBox | undefined, attributes?: Record<string, unknown>) => void
  onShow: (ref: BoxRef) => void
}

const shown = (value: unknown) => (Array.isArray(value) ? value.join(', ') : String(value))

// "Caixa" de um objeto (ex.: o Veículo 1 é a caixa 1. motorcycle). Escolher
// a caixa a mostra na imagem e preenche o que a label e os atributos dela
// dizem. Os atributos da caixa aparecem aqui: o objeto e a caixa são um só.
export function BoxLink({ type, object, sets, taken, onLink, onShow }: Props) {
  const options = candidates(type, sets)
  const linked = object.box ? findLinked(sets, object.box) : null
  const value = linked ? `${linked.task}#${linked.index}` : object.box ? 'lost' : ''
  const implied = object.box ? type.box_labels?.find((label) => label.label === object.box?.label)?.fields ?? {} : {}
  const box = linked ? sets.find((s) => s.task === linked.task)?.boxes[linked.index] : undefined
  const notes = boxAttributeNotes(type, object, box?.attributes)
  const others = notes.filter((note) => !note.field)
  const differs = notes.filter((note) => note.differs)

  function choose(key: string) {
    if (key === '') return onLink(undefined)
    const found = options.find((option) => option.key === key)
    if (!found) return
    onLink(toObjectBox(found.ref.task, found.box), found.box.attributes)
    onShow(found.ref)
  }

  if (options.length === 0 && !object.box) {
    return <p className="box-link box-link--empty">Caixa: nenhuma caixa {(type.box_labels ?? []).map((label) => label.label).join(' / ')} nesta imagem.</p>
  }
  return (
    <div className={value === 'lost' ? 'box-link box-link--lost' : 'box-link'}>
      <label>
        <span>Caixa</span>
        <select onChange={(event) => choose(event.target.value)} value={value}>
          <option value="">Sem caixa</option>
          {value === 'lost' && <option value="lost">{object.box?.label} (não achada: a tarefa foi importada de novo?)</option>}
          {options.map((option) => (
            <option disabled={taken.has(option.key) && option.key !== value} key={option.key} value={option.key}>
              {option.name}{taken.has(option.key) && option.key !== value ? ' (de outro objeto)' : ''}
            </option>
          ))}
        </select>
      </label>
      {linked && <button className="button button--secondary button--sm" onClick={() => onShow(linked)} type="button">Ver na imagem</button>}
      {Object.keys(implied).length > 0 && (
        <small>
          A caixa {object.box?.label} já diz:{' '}
          {Object.entries(implied).map(([key, v]) => {
            const field = type.fields.find((f) => f.key === key)
            return field ? `${fieldLabel(field).toLowerCase()} = ${optionLabel(field, String(v))}` : key
          }).join(', ')}
        </small>
      )}
      {others.length > 0 && (
        <small>
          Atributos da caixa (CVAT): {others.map((note) => `${note.attr} = ${shown(note.value)}`).join(', ')}
        </small>
      )}
      {differs.map((note) => (
        <small className="box-link__differs" key={note.attr}>
          {fieldLabel(note.field!)}: no CVAT a caixa está "{displayValue(note.field!, note.value)}" (atributo {note.attr}). Vale o daqui; o CVAT não muda.
        </small>
      ))}
    </div>
  )
}
