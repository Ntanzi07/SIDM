import { useRef, useState } from 'react'
import type { FolderField } from '../../services/api'

// Os campos do padrão antigo primeiro, na ordem dele; depois os outros, na
// ordem do backend (arquivo, imagem, objetos).
const ORDER = [
  'sha1', 'direction', 'resolution', 'capture_angle', 'day_time', 'vehicle_type', 'image_range',
  'vehicle_plate_type', 'vehicle_plate_category', 'weather', 'condition', 'vehicle_plate',
]

type Mode = 'v2' | 'legacy' | 'custom'

// O nome dos arquivos de saída: o do SIDM (v2), o padrão antigo (hash +
// características + placa; só no perfil veiculos) ou um modelo próprio com
// os campos do perfil de anotação entre chaves.
export function FileNameTemplate({ value, fields, legacy, onChange }: {
  value: string
  fields: FolderField[]
  legacy: string
  onChange: (value: string) => void
}) {
  const input = useRef<HTMLInputElement>(null)
  // "Personalizado" escolhido fica, mesmo com o texto vazio ou igual ao antigo.
  // (o componente é remontado a cada perfil aberto: key no ExportPage)
  const [custom, setCustom] = useState(false)
  const mode: Mode = custom ? 'custom' : value === '' ? 'v2' : legacy && value === legacy ? 'legacy' : 'custom'
  const offered = fields.filter((field) => field.name && !field.alias)
  const sorted = offered
    .map((field, i) => ({ field, rank: (ORDER.indexOf(field.field) + 1 || 99) * 1000 + i }))
    .sort((a, b) => a.rank - b.rank)
    .map(({ field }) => field)
  // Campos do modelo que o perfil de anotação não tem (ex.: trocou o perfil).
  const known = new Set(fields.filter((field) => field.name).map((field) => field.field))
  const unknown = [...value.matchAll(/\{([^{}]*)\}/g)].map((m) => m[1]).filter((field) => !known.has(field))
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
    ['v2', 'Nome do SIDM', 'o nome guardado no banco (v2 no perfil veiculos; sha1 nos outros)'],
    ...(legacy ? [['legacy', 'Padrão antigo', 'hash + características + placa: …3a3fF01LDBEB1S01KOY7386.jpg'] as [Mode, string, string]] : []),
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
                onChange(key === 'v2' ? '' : key === 'legacy' ? legacy : value || legacy || '{sha1}')
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
              <button className="file-name__chip" key={field.field} onClick={() => insert(field.field)} title={`{${field.field}}`} type="button">
                {field.label}
              </button>
            ))}
          </div>
          {missingHash && <p className="form-error">Use {'{sha1}'} (ou {'{name}'}) no nome, para cada imagem ter um nome diferente.</p>}
          {unknown.length > 0 && (
            <p className="form-error">O perfil de anotação não tem: {unknown.map((field) => `{${field}}`).join(' ')}.</p>
          )}
        </>
      )}
    </div>
  )
}
