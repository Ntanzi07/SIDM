import { imageFileUrl, type BoxSet } from '../../services/api'
import './ImageWithBoxes.css'

// Cor fixa por label (a mesma label tem sempre a mesma cor).
const BOX_COLORS = ['#e11d48', '#2563eb', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d']
function colorOf(label: string): string {
  let hash = 0
  for (const char of label) hash = (hash * 31 + char.charCodeAt(0)) | 0
  return BOX_COLORS[Math.abs(hash) % BOX_COLORS.length]
}

// A imagem com as caixas do CVAT por cima (só para conferir), de todas as
// tarefas em que ela está. As caixas são em pixels da imagem no CVAT: viram
// porcentagem para acompanhar o tamanho em que a imagem aparece na tela.
export function ImageWithBoxes({ sha1, alt, sets, show }: { sha1: string; alt: string; sets: BoxSet[]; show: boolean }) {
  const boxes = sets
    .filter((set) => set.width > 0 && set.height > 0)
    .flatMap((set) => set.boxes.map((box) => ({ box, set })))
  return (
    <div className="annotation-boxes-frame">
      <img alt={alt} src={imageFileUrl(sha1)} />
      {show && boxes.map(({ box, set }, i) => (
        <div
          className="annotation-box"
          key={i}
          style={{
            left: `${(box.x / set.width) * 100}%`,
            top: `${(box.y / set.height) * 100}%`,
            width: `${(box.w / set.width) * 100}%`,
            height: `${(box.h / set.height) * 100}%`,
            borderColor: colorOf(box.label),
            borderStyle: box.occluded ? 'dashed' : 'solid',
            transform: box.rotation ? `rotate(${box.rotation}deg)` : undefined,
          }}
          title={[sets.length > 1 ? `${set.task}: ${box.label}` : box.label, box.occluded ? 'oculta' : '', ...Object.entries(box.attributes ?? {}).map(([k, v]) => `${k}: ${String(v)}`)].filter(Boolean).join(' · ')}
        >
          <span style={{ background: colorOf(box.label) }}>{box.label}</span>
        </div>
      ))}
    </div>
  )
}
