import { annotationOptions } from '../../data/annotationOptions'
import type { FolderField, FolderLevel } from '../../services/api'

const FIELD_LABELS: Record<string, string> = {
  vehicle_type: 'Tipo de veículo',
  plate_type: 'Tipo de placa',
  plate_category: 'Categoria da placa',
  vehicle_color: 'Cor do veículo',
  resolution: 'Resolução',
  direction: 'Direção',
  day_time: 'Período',
  weather: 'Clima',
  capture_angle: 'Ângulo de captura',
  image_range: 'Alcance',
  equipment: 'Equipamento',
  camera: 'Câmera',
  package: 'Pacote',
  split: 'Divisão (train/valid/test)',
}

// Rótulo em português de cada valor (para a tabela de nomes).
const OPTION_KEYS: Record<string, keyof typeof annotationOptions> = {
  vehicle_type: 'type',
  vehicle_color: 'color',
  plate_type: 'plate_type',
  plate_category: 'plate_category',
  direction: 'direction',
  day_time: 'day_time',
  weather: 'weather',
  capture_angle: 'capture_angle',
  image_range: 'image_range',
}

function valueLabel(field: string, value: string): string {
  const key = OPTION_KEYS[field]
  const options = key ? (annotationOptions[key] as readonly (readonly [string, string])[]) : []
  return options.find(([v]) => v === value)?.[1] ?? value
}

// Nome final da pasta de um valor (o que o backend vai usar).
function folderNameOf(level: FolderLevel, value: string, field: FolderField | undefined): string {
  return level.names?.[value] || field?.values?.[value] || value
}

// Valores na ordem do nome padrão da pasta (a resolução fica 01, 02, 03…).
function sortedValues(values: Record<string, string>): string[] {
  return Object.keys(values).sort((a, b) => values[a].localeCompare(values[b]))
}

const BAD_CHARS = /[/\\:*?"<>|]/

// Os níveis da árvore de pastas do VOC: de qual metadado sai cada nível e
// como cada valor vira nome de pasta.
export function FolderLevels({ levels, fields, unknownDefault, onChange }: {
  levels: FolderLevel[]
  fields: FolderField[]
  unknownDefault: string
  onChange: (levels: FolderLevel[]) => void
}) {
  const byName = new Map(fields.map((field) => [field.field, field]))
  const unused = fields.filter((field) => !levels.some((level) => level.field === field.field))

  const update = (index: number, patch: Partial<FolderLevel>) =>
    onChange(levels.map((level, i) => (i === index ? { ...level, ...patch } : level)))
  const move = (index: number, delta: number) => {
    const next = [...levels]
    const [item] = next.splice(index, 1)
    next.splice(index + delta, 0, item)
    onChange(next)
  }
  const setName = (index: number, value: string, name: string) => {
    const names = { ...(levels[index].names ?? {}) }
    if (name.trim()) names[value] = name
    else delete names[value]
    update(index, { names })
  }

  // Exemplo de caminho: o primeiro valor de cada nível.
  const example = levels.map((level) => {
    const field = byName.get(level.field)
    const first = field?.values ? sortedValues(field.values)[0] : undefined
    return first ? folderNameOf(level, first, field) : `<${FIELD_LABELS[level.field] ?? level.field}>`
  })

  return (
    <div className="folder-levels">
      {levels.length > 0 && (
        <p className="folder-levels__example">
          Exemplo: <code>{example.join(' / ')} / imagem.jpg + imagem.xml</code>
        </p>
      )}
      {levels.map((level, index) => {
        const field = byName.get(level.field)
        const values = field?.values ? sortedValues(field.values) : null
        return (
          <div className="folder-level" key={level.field}>
            <div className="folder-level__head">
              <strong>{index + 1}. {FIELD_LABELS[level.field] ?? level.field}</strong>
              <span className="export-actions__spacer" />
              <button aria-label="Subir" className="button button--secondary button--sm" disabled={index === 0} onClick={() => move(index, -1)} type="button">↑</button>
              <button aria-label="Descer" className="button button--secondary button--sm" disabled={index === levels.length - 1} onClick={() => move(index, 1)} type="button">↓</button>
              <button aria-label="Tirar nível" className="button button--danger button--sm" onClick={() => onChange(levels.filter((_, i) => i !== index))} type="button">✕</button>
            </div>
            {values ? (
              <table className="export-labels folder-level__names">
                <thead><tr><th>Valor</th><th>Nome da pasta</th></tr></thead>
                <tbody>
                  {values.map((value) => {
                    const name = level.names?.[value] ?? ''
                    return (
                      <tr key={value}>
                        <td>{valueLabel(level.field, value)} <small>({value})</small></td>
                        <td>
                          <input
                            className={BAD_CHARS.test(name) ? 'folder-level__bad' : undefined}
                            onChange={(event) => setName(index, value, event.target.value)}
                            placeholder={field?.values?.[value] ?? value}
                            value={name}
                          />
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            ) : (
              <p className="profile-name-hint">
                {level.field === 'split' ? 'As pastas train, valid e test, pela divisão do perfil.' : 'A pasta é o próprio valor (caracteres inválidos viram _).'}
              </p>
            )}
            {level.field !== 'split' && (
              <label className="folder-level__unknown">
                <span>Imagens sem esse dado vão para</span>
                <input
                  className={BAD_CHARS.test(level.unknown ?? '') ? 'folder-level__bad' : undefined}
                  onChange={(event) => update(index, { unknown: event.target.value })}
                  placeholder={unknownDefault}
                  value={level.unknown ?? ''}
                />
              </label>
            )}
          </div>
        )
      })}
      {unused.length > 0 && (
        <select
          aria-label="Adicionar nível"
          className="folder-levels__add"
          onChange={(event) => {
            if (event.target.value) onChange([...levels, { field: event.target.value }])
          }}
          value=""
        >
          <option value="">+ Adicionar nível de pasta…</option>
          {unused.map((field) => <option key={field.field} value={field.field}>{FIELD_LABELS[field.field] ?? field.field}</option>)}
        </select>
      )}
      <p className="profile-name-hint">
        Os dados do veículo vêm do primeiro veículo da imagem. Nome de pasta não pode ter / \ : * ? " &lt; &gt; |.
      </p>
    </div>
  )
}
