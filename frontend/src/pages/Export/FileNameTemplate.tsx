import { useRef, useState } from 'react'

const FIELD_LABELS: Record<string, string> = {
  sha1: 'Hash (sha1)',
  name: 'Nome do SIDM (v2)',
  original: 'Nome original',
  package: 'Pacote',
  equipment: 'Equipamento',
  camera: 'Câmera',
  captured_at: 'Data e hora',
  date: 'Data',
  plate: 'Placa',
  direction: 'Direção',
  resolution: 'Resolução',
  capture_angle: 'Ângulo',
  day_time: 'Período',
  vehicle_type: 'Tipo de veículo',
  image_range: 'Alcance',
  plate_type: 'Tipo de placa',
  plate_category: 'Categoria da placa',
  weather: 'Clima',
  condition: 'Condição',
  vehicle_color: 'Cor do veículo',
}

// Os campos na ordem do padrão antigo, depois os outros.
const ORDER = [
  'sha1', 'direction', 'resolution', 'capture_angle', 'day_time', 'vehicle_type', 'image_range',
  'plate_type', 'plate_category', 'weather', 'condition', 'plate',
  'vehicle_color', 'captured_at', 'date', 'name', 'original', 'package', 'equipment', 'camera',
]

type Mode = 'v2' | 'legacy' | 'custom'

// O nome dos arquivos de saída: o do SIDM (v2), o padrão antigo (hash +
// características + placa) ou um modelo próprio com campos entre chaves.
export function FileNameTemplate({ value, fields, legacy, onChange }: {
  value: string
  fields: string[]
  legacy: string
  onChange: (value: string) => void
}) {
  const input = useRef<HTMLInputElement>(null)
  // "Personalizado" escolhido fica, mesmo com o texto vazio ou igual ao antigo.
  // (o componente é remontado a cada perfil aberto: key no ExportPage)
  const [custom, setCustom] = useState(false)
  const mode: Mode = custom ? 'custom' : value === '' ? 'v2' : value === legacy ? 'legacy' : 'custom'
  const sorted = [...fields].sort((a, b) => (ORDER.indexOf(a) + 1 || 99) - (ORDER.indexOf(b) + 1 || 99))
  const missingHash = (mode === 'custom' || value !== '') && !value.includes('{sha1}') && !value.includes('{name}')

  // Insere o campo onde está o cursor (ou no fim).
  function insert(field: string) {
    const el = input.current
    const token = `{${field}}`
    const start = el?.selectionStart ?? value.length
    const end = el?.selectionEnd ?? value.length
    onChange(value.slice(0, start) + token + value.slice(end))
    requestAnimationFrame(() => {
      el?.focus()
      el?.setSelectionRange(start + token.length, start + token.length)
    })
  }

  const options: Array<[Mode, string, string]> = [
    ['v2', 'Nome do SIDM (v2)', 'o nome guardado: sha1_v2_equipamento_câmera_…'],
    ['legacy', 'Padrão antigo', 'hash + características + placa: …3a3fF01LDBEB1S01KOY7386.jpg'],
    ['custom', 'Personalizado', 'monte o nome com os campos'],
  ]

  return (
    <div className="file-name">
      <div className="export-formats">
        {options.map(([key, title, hint]) => (
          <label className={mode === key ? 'export-format export-format--active' : 'export-format'} key={key}>
            <input
              checked={mode === key}
              name="file-name-mode"
              onChange={() => {
                setCustom(key === 'custom')
                onChange(key === 'v2' ? '' : key === 'legacy' ? legacy : value || legacy)
              }}
              type="radio"
            />
            <strong>{title}</strong>
            <span>{hint}</span>
          </label>
        ))}
      </div>
      {mode !== 'v2' && (
        <>
          <input
            aria-label="Modelo do nome"
            className={missingHash ? 'file-name__input file-name__input--bad' : 'file-name__input'}
            onChange={(event) => onChange(event.target.value)}
            ref={input}
            spellCheck={false}
            value={value}
          />
          <div className="file-name__chips">
            {sorted.map((field) => (
              <button className="file-name__chip" key={field} onClick={() => insert(field)} title={`{${field}}`} type="button">
                {FIELD_LABELS[field] ?? field}
              </button>
            ))}
          </div>
          {missingHash && <p className="form-error">Use {'{sha1}'} (ou {'{name}'}) no nome, para cada imagem ter um nome diferente.</p>}
          <p className="profile-name-hint">
            Os campos saem com o código do nome v2 (direção F/T, resolução 01…10, …). Campo sem valor vira "_" na largura dele.
            Os do veículo vêm do primeiro veículo; a condição é a primeira da imagem. A extensão é a do arquivo.
            O exemplo do nome aparece na prévia.
          </p>
        </>
      )}
    </div>
  )
}
