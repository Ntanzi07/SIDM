import { useEffect, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { annotationOptions } from '../../data/annotationOptions'
import { getImageBoxes, imageFileUrl, searchCameras, searchEquipments, type BoxSet, type PageResponse } from '../../services/api'
import { useAnnotator } from '../../hooks/useAnnotator'
import type { AnnotationForm } from '../../types/annotation'
import './Annotation.css'

type SelectFieldProps = {
  label: string
  value: string
  options: readonly (readonly [string, string])[]
  placeholder?: string // com placeholder, o campo pode ficar vazio
  onChange: (value: string) => void
}

function SelectField({ label, value, options, placeholder, onChange }: SelectFieldProps) {
  // Valor que não está na lista (ex.: veio de um import sem tradução). Sem
  // isto, o navegador mostraria a primeira opção, e escolher essa opção não
  // dispararia mudança: o valor errado ficaria preso.
  // O mesmo vale para o valor VAZIO: sem uma opção vazia, o navegador
  // mostraria a primeira da lista, mas o valor continuaria "".
  const invalid = value !== '' && !options.some(([option]) => option === value)
  const empty = value === ''
  return (
    <label className={invalid || empty ? 'annotation-field annotation-field--invalid' : 'annotation-field'}>
      <span>{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {invalid && <option value={value}>"{value}" (inválido: escolha outro)</option>}
        {(empty || placeholder !== undefined) && <option disabled={!empty} value="">{placeholder ?? 'Selecione'}</option>}
        {options.map(([value, name]) => <option key={value} value={value}>{name}</option>)}
      </select>
    </label>
  )
}

type AutocompleteFieldProps = {
  label: string
  value: string
  search: (q: string) => Promise<PageResponse<string>>
  placeholder?: string
  onChange: (value: string) => void
}

// Combobox próprio (o <datalist> do navegador não atualiza a lista aberta
// quando as sugestões chegam depois, e filtra demais quando já há um valor).
// Ao focar, mostra a lista toda; ao digitar, busca no backend (300 ms depois
// da última tecla). Setas + Enter escolhem, Esc fecha.
function AutocompleteField({ label, value, search, placeholder, onChange }: AutocompleteFieldProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [suggestions, setSuggestions] = useState<string[]>([])
  const [active, setActive] = useState(-1)

  useEffect(() => {
    if (!open) return
    let alive = true
    const timer = window.setTimeout(() => {
      search(query)
        .then((page) => {
          if (!alive) return
          setSuggestions(page.items)
          setActive(-1)
        })
        .catch(() => { if (alive) setSuggestions([]) })
    }, query ? 300 : 0)
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
  }, [open, query, search])

  function choose(name: string) {
    onChange(name)
    setOpen(false)
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setOpen(true)
      setActive((i) => Math.min(i + 1, suggestions.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (event.key === 'Enter' && open && active >= 0 && suggestions[active]) {
      event.preventDefault()
      choose(suggestions[active])
    } else if (event.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <label className="annotation-field annotation-combobox">
      <span>{label}</span>
      <input
        autoComplete="off"
        onBlur={() => setOpen(false)}
        onChange={(event) => {
          onChange(event.target.value)
          setQuery(event.target.value)
          setOpen(true)
        }}
        onFocus={() => {
          setQuery('') // ao entrar no campo, mostra todos (não só o que bate com o valor atual)
          setOpen(true)
        }}
        onKeyDown={handleKeyDown}
        placeholder={placeholder ?? 'Digite para buscar'}
        role="combobox"
        aria-expanded={open}
        value={value}
      />
      {open && suggestions.length > 0 && (
        <ul className="annotation-combobox__list" role="listbox">
          {suggestions.map((name, i) => (
            <li
              aria-selected={name === value}
              className={i === active ? 'annotation-combobox__item annotation-combobox__item--active' : 'annotation-combobox__item'}
              key={name}
              // mousedown (e não click): acontece antes do blur que fecha a lista
              onMouseDown={(event) => {
                event.preventDefault()
                choose(name)
              }}
              role="option"
            >
              {name}
            </li>
          ))}
        </ul>
      )}
      {open && query !== '' && suggestions.length === 0 && (
        <div className="annotation-combobox__list annotation-combobox__empty">
          Nenhum cadastrado com "{query}". Ao salvar, a tela pergunta se quer cadastrar.
        </div>
      )}
    </label>
  )
}

// Cor fixa por label (a mesma label tem sempre a mesma cor).
const BOX_COLORS = ['#e11d48', '#2563eb', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d']
function colorOf(label: string): string {
  let hash = 0
  for (const char of label) hash = (hash * 31 + char.charCodeAt(0)) | 0
  return BOX_COLORS[Math.abs(hash) % BOX_COLORS.length]
}

// A imagem com as caixas do CVAT por cima (só para conferir). As caixas são
// em pixels da imagem no CVAT: viram porcentagem para acompanhar o tamanho
// em que a imagem aparece na tela.
function ImageWithBoxes({ sha1, alt, set, show }: { sha1: string; alt: string; set: BoxSet | null; show: boolean }) {
  return (
    <div className="annotation-boxes-frame">
      <img alt={alt} src={imageFileUrl(sha1)} />
      {show && set && set.width > 0 && set.height > 0 && set.boxes.map((box, i) => (
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
          title={[box.label, box.occluded ? 'oculta' : '', ...Object.entries(box.attributes ?? {}).map(([k, v]) => `${k}: ${String(v)}`)].filter(Boolean).join(' · ')}
        >
          <span style={{ background: colorOf(box.label) }}>{box.label}</span>
        </div>
      ))}
    </div>
  )
}

// As caixas da imagem aberta (null: sem caixas, ou ainda carregando).
function useImageBoxes(sha1: string | undefined): BoxSet | null {
  const [loaded, setLoaded] = useState<{ sha1?: string; set: BoxSet | null }>({ set: null })
  useEffect(() => {
    if (!sha1) return
    let active = true
    getImageBoxes(sha1)
      .then((set) => { if (active) setLoaded({ sha1, set }) })
      .catch(() => { if (active) setLoaded({ sha1, set: null }) })
    return () => { active = false }
  }, [sha1])
  return loaded.sha1 === sha1 ? loaded.set : null
}

function formatSize(bytes: number) {
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

export function AnnotationPage() {
  const navigate = useNavigate()
  const { id } = useParams()
  const packageName = decodeURIComponent(id ?? '')
  const annotator = useAnnotator(packageName)
  const { image, form, codes } = annotator

  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [showBoxes, setShowBoxes] = useState(true)
  const boxSet = useImageBoxes(annotator.image?.sha1)

  const update = <K extends keyof AnnotationForm>(field: K, value: AnnotationForm[K]) => annotator.updateForm(field, value)

  async function handleDeleteImage() {
    const ok = await annotator.deleteCurrentImage()
    if (ok) setShowDeleteModal(false)
  }

  if (annotator.loading) return <section className="annotation-page annotation-empty">Carregando a fila...</section>
  if (annotator.error && !image && !annotator.finished) {
    return (
      <section className="annotation-page annotation-empty">
        <h1>Não foi possível abrir a anotação</h1>
        <p>{annotator.error}</p>
        <button className="button button--secondary" onClick={() => navigate(`/pacotes/${encodeURIComponent(packageName)}`)} type="button">Voltar</button>
      </section>
    )
  }

  return (
    <section className="annotation-page">
      {/* Delete image confirmation modal */}
      {showDeleteModal && (
        <div className="modal-backdrop" onClick={() => setShowDeleteModal(false)}>
          <div className="modal-dialog" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <strong>Excluir imagem</strong>
              <button className="modal-close" onClick={() => setShowDeleteModal(false)} type="button">✕</button>
            </div>
            <div className="modal-body">
              <p className="modal-warning">Esta ação vai excluir permanentemente a imagem <strong>{image?.original_name}</strong> e todos os seus dados de anotação. Esta ação não pode ser desfeita.</p>
            </div>
            <div className="modal-actions">
              <button className="button button--secondary" onClick={() => setShowDeleteModal(false)} type="button">Cancelar</button>
              <button className="button button--danger" disabled={annotator.saving} onClick={() => void handleDeleteImage()} type="button">
                {annotator.saving ? 'Excluindo...' : 'Excluir imagem'}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="annotation-header">
        <button className="back-button" onClick={() => navigate(`/pacotes/${encodeURIComponent(packageName)}`)} type="button">← Voltar para {packageName}</button>
        <div>
          <strong>{annotator.progress.done} de {annotator.progress.total} concluídas</strong>
          <span>{annotator.progress.total ? Math.round((annotator.progress.done / annotator.progress.total) * 100) : 0}%</span>
        </div>
      </div>

      <div className="annotation-layout">
        <div className="annotation-navigation">
          <button className="button button--secondary" disabled={!annotator.canGoBack || annotator.saving} onClick={() => void annotator.previous()} type="button">← Anterior</button>
          <span>
            {annotator.revisiting
              ? 'Revendo uma imagem anterior'
              : `${annotator.progress.pending.toLocaleString('pt-BR')} pendente(s) na fila`}
          </span>
          {annotator.revisiting ? (
            <button className="button button--secondary" disabled={annotator.saving} onClick={() => void annotator.forward()} type="button">Seguinte →</button>
          ) : (
            <button
              className="button button--secondary"
              disabled={!image || annotator.saving}
              onClick={() => void annotator.skip()}
              title="A imagem volta para o fim da fila"
              type="button"
            >
              Pular →
            </button>
          )}
        </div>

        <main className="annotation-main">
          {image ? (
            <div className="annotation-workspace">
              <div className="annotation-preview-column">
                <div className="annotation-preview-card">
                  <div className="annotation-preview">
                    <ImageWithBoxes alt={image.original_name} set={boxSet} sha1={image.sha1} show={showBoxes} />
                  </div>
                  <div className="annotation-image-meta">
                    <div>
                      <strong>{image.original_name}</strong>
                      <span>
                        {formatSize(image.size)} · {image.ext}
                        {boxSet && boxSet.boxes.length > 0 && (
                          <label className="annotation-boxes-toggle">
                            <input checked={showBoxes} onChange={(event) => setShowBoxes(event.target.checked)} type="checkbox" />
                            {boxSet.boxes.length} caixa(s) do CVAT
                          </label>
                        )}
                      </span>
                    </div>
                    <button
                      className="button button--danger button--sm"
                      disabled={annotator.saving}
                      onClick={() => setShowDeleteModal(true)}
                      type="button"
                    >
                      Excluir imagem
                    </button>
                  </div>
                </div>
              </div>

              <div className="annotation-form-card">
                <div className="annotation-form-heading">
                  <div>
                    <h2>Dados da anotação</h2>
                    <p>Preencha os campos de acordo com o que aparece na imagem.</p>
                  </div>
                  <span className={image.status === 'verified' ? 'annotation-status annotation-status--done' : 'annotation-status'}>
                    {image.status === 'verified' ? 'Concluída' : 'Pendente'}
                  </span>
                </div>

                {annotator.error && <p className="annotation-feedback annotation-feedback--error">{annotator.error}</p>}
                {annotator.message && <p className="annotation-feedback">{annotator.message}</p>}

                <div className="annotation-fields">
                  <AutocompleteField
                    label="Equipamento"
                    onChange={(value) => update('equipment', value)}
                    placeholder="Digite o equipamento"
                    search={searchEquipments}
                    value={form.equipment}
                  />
                  <AutocompleteField
                    label="Câmera"
                    onChange={(value) => update('camera', value)}
                    placeholder="Digite a câmera"
                    search={searchCameras}
                    value={form.camera}
                  />
                  <SelectField label="Direção" value={form.direction} options={annotationOptions.direction} onChange={(value) => update('direction', value)} />
                  <label className="annotation-field">
                    <span>Resolução</span>
                    <select value={form.resolution} onChange={(event) => update('resolution', event.target.value)}>
                      <option value="">Selecione</option>
                      {Object.values(codes.resolution ?? {}).sort((a, b) => Number.parseInt(a, 10) - Number.parseInt(b, 10)).map((size) => <option key={size} value={size}>{size}</option>)}
                    </select>
                  </label>
                  <SelectField label="Ângulo" value={form.capture_angle} options={annotationOptions.capture_angle} onChange={(value) => update('capture_angle', value)} />
                  <SelectField label="Período" value={form.day_time} options={annotationOptions.day_time} onChange={(value) => update('day_time', value)} />
                  <SelectField label="Alcance" value={form.image_range} options={annotationOptions.image_range} onChange={(value) => update('image_range', value)} />
                  <SelectField label="Clima" value={form.weather} options={annotationOptions.weather} onChange={(value) => update('weather', value)} />
                  <label className="annotation-field">
                    <span>Data e hora da captura</span>
                    <input step="1" type="datetime-local" value={form.captured_at} onChange={(event) => update('captured_at', event.target.value)} />
                  </label>
                </div>

                <p className="annotation-hint">
                  Equipamento ou câmera novo: digite o nome e salve; a tela pede para confirmar o cadastro.
                  Para renomear ou desativar, use <Link to="/configurar">Configurar</Link>.
                </p>

                <label className="annotation-field annotation-condition">
                  <span>Condição</span>
                  <select value={form.conditions[0] ?? 'no_damage'} onChange={(event) => update('conditions', [event.target.value])}>
                    {annotationOptions.condition.map(([value, name]) => <option key={value} value={value}>{name}</option>)}
                  </select>
                </label>

                {/* Vehicle rows */}
                {form.vehicles.map((vehicle, index) => (
                  <div className="vehicle-row" key={index}>
                    <div className="vehicle-row-header">
                      <h3>Veículo {index + 1}</h3>
                      {form.vehicles.length > 1 && (
                        <button
                          className="button button--danger button--sm"
                          onClick={() => annotator.removeVehicle(index)}
                          type="button"
                        >
                          Remover
                        </button>
                      )}
                    </div>
                    <div className="annotation-fields">
                      <label className="annotation-field">
                        <span>Placa</span>
                        <input required value={vehicle.plate} onChange={(event) => annotator.updateVehicle(index, { plate: event.target.value.toUpperCase() })} />
                      </label>
                      <SelectField label="Tipo" value={vehicle.type} options={annotationOptions.type} onChange={(value) => annotator.updateVehicle(index, { type: value })} />
                      <SelectField label="Tipo da placa" value={vehicle.plate_type} options={annotationOptions.plate_type} onChange={(value) => annotator.updateVehicle(index, { plate_type: value })} />
                      <SelectField label="Categoria" value={vehicle.plate_category} options={annotationOptions.plate_category} onChange={(value) => annotator.updateVehicle(index, { plate_category: value })} />
                      <SelectField label="Cor" value={vehicle.color} options={annotationOptions.color} placeholder="Selecione" onChange={(value) => annotator.updateVehicle(index, { color: value })} />
                    </div>
                  </div>
                ))}

                <button
                  className="button button--secondary annotation-add-vehicle"
                  onClick={annotator.addVehicle}
                  type="button"
                >
                  + Adicionar veículo
                </button>

                <button className="button button--primary annotation-save" disabled={annotator.saving} onClick={() => void annotator.save()} type="button">
                  {annotator.saving ? 'Salvando...' : 'Salvar anotação'}
                </button>
              </div>
            </div>
          ) : (
            <div className="annotation-empty">
              <h2>Nada pendente na fila</h2>
              <p>
                {annotator.progress.total === 0
                  ? 'Este pacote ainda não tem imagens.'
                  : 'Todas as imagens livres já foram anotadas. Se alguém está com uma reservada, ela volta para a fila em até 15 minutos.'}
              </p>
              {annotator.message && <p className="annotation-feedback">{annotator.message}</p>}
            </div>
          )}
        </main>
      </div>
    </section>
  )
}
