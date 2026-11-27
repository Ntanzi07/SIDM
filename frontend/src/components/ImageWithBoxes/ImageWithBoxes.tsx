import { imageFileUrl, type BoxSet } from '../../services/api'
import { colorOf, inkOn, type BoxRef } from '../../utils/boxes'
import { useTaskColors } from '../../utils/taskColors'
import './ImageWithBoxes.css'

// A imagem com as caixas do CVAT por cima, de todas as tarefas em que ela
// está. As caixas são em pixels da imagem no CVAT: viram porcentagem para
// acompanhar o tamanho em que a imagem aparece na tela. Com onSelect, clicar
// numa caixa seleciona (para ver os atributos); pending marca as caixas com
// atributo obrigatório vazio.
export function ImageWithBoxes({ sha1, alt, sets, show, selected, onSelect, pending }: {
  sha1: string
  alt: string
  sets: BoxSet[]
  show: boolean
  selected?: BoxRef | null
  onSelect?: (box: BoxRef) => void
  pending?: (box: BoxRef) => boolean
}) {
  useTaskColors()
  const boxes = sets
    .filter((set) => set.width > 0 && set.height > 0)
    .flatMap((set) => set.boxes.map((box, index) => ({ box, set, index })))
  return (
    <div className="annotation-boxes-frame">
      <img alt={alt} src={imageFileUrl(sha1)} />
      {show && boxes.map(({ box, set, index }) => {
        const ref = { task: set.task, index }
        const isSelected = selected?.task === set.task && selected.index === index
        const isPending = pending?.(ref) ?? false
        return (
          <div
            className={[
              'annotation-box',
              onSelect ? 'annotation-box--clickable' : '',
              isSelected ? 'annotation-box--selected' : '',
            ].filter(Boolean).join(' ')}
            key={`${set.task}-${index}`}
            onClick={onSelect ? () => onSelect(ref) : undefined}
            style={{
              left: `${(box.x / set.width) * 100}%`,
              top: `${(box.y / set.height) * 100}%`,
              width: `${(box.w / set.width) * 100}%`,
              height: `${(box.h / set.height) * 100}%`,
              borderColor: colorOf(box.label, set.task),
              borderStyle: box.occluded ? 'dashed' : 'solid',
              transform: box.rotation ? `rotate(${box.rotation}deg)` : undefined,
            }}
            title={[sets.length > 1 ? `${set.task}: ${box.label}` : box.label, box.occluded ? 'oculta' : '', isPending ? 'atributo obrigatório faltando' : '', ...Object.entries(box.attributes ?? {}).map(([k, v]) => `${k}: ${String(v)}`)].filter(Boolean).join(' · ')}
          >
            <span style={{ background: colorOf(box.label, set.task), color: inkOn(colorOf(box.label, set.task)) }}>{box.label}{isPending ? ' !' : ''}</span>
          </div>
        )
      })}
    </div>
  )
}
