import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { annotationOptions } from '../../data/annotationOptions'
import { imageFileUrl } from '../../services/api'
import { useAnnotator } from '../../hooks/useAnnotator'
import type { AnnotationForm } from '../../types/annotation'
import './Annotation.css'

type SelectFieldProps = {
  label: string
  value: string
  options: readonly (readonly [string, string])[]
  onChange: (value: string) => void
}

function SelectField({ label, value, options, onChange }: SelectFieldProps) {
  return (
    <label className="annotation-field">
      <span>{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map(([code, name]) => <option key={code} value={code}>{code} — {name}</option>)}
      </select>
    </label>
  )
}

type ComboFieldProps = {
  label: string
  value: string
  listId: string
  suggestions: string[]
  placeholder?: string
  onChange: (value: string) => void
}

function ComboField({ label, value, listId, suggestions, placeholder, onChange }: ComboFieldProps) {
  return (
    <label className="annotation-field">
      <span>{label}</span>
      <input
        list={listId}
        placeholder={placeholder ?? 'Digite ou selecione'}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <datalist id={listId}>
        {suggestions.map((s) => <option key={s} value={s} />)}
      </datalist>
    </label>
  )
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

  const update = <K extends keyof AnnotationForm>(field: K, value: AnnotationForm[K]) => annotator.updateForm(field, value)
  const equipmentOptions = codes.equipments ?? []
  const cameraOptions = codes.cameras ?? []

  async function handleDeleteImage() {
    const ok = await annotator.deleteCurrentImage()
    if (ok) setShowDeleteModal(false)
  }

  if (annotator.loading) return <section className="annotation-page annotation-empty">Carregando imagens...</section>
  if (annotator.error && !image && annotator.images.length === 0) {
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
          <button className="button button--secondary" disabled={annotator.selectedIndex === 0} onClick={() => annotator.setSelectedIndex((current) => current - 1)} type="button">← Anterior</button>
          <span>Imagem {annotator.selectedIndex + 1} de {annotator.images.length}</span>
          <button className="button button--secondary" disabled={annotator.selectedIndex === annotator.images.length - 1} onClick={() => annotator.setSelectedIndex((current) => current + 1)} type="button">Próxima →</button>
        </div>

        <main className="annotation-main">
          {image ? (
            <div className="annotation-workspace">
              <div className="annotation-preview-column">
                <div className="annotation-preview-card">
                  <div className="annotation-preview">
                    <img alt={image.original_name} src={imageFileUrl(image.sha1)} />
                  </div>
                  <div className="annotation-image-meta">
                    <div>
                      <strong>{image.original_name}</strong>
                      <span>{formatSize(image.size)} · {image.ext}</span>
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
                  <ComboField
                    label="Equipamento"
                    listId="equipment-list"
                    onChange={(value) => update('equipment', value)}
                    placeholder="Digite o equipamento"
                    suggestions={equipmentOptions}
                    value={form.equipment}
                  />
                  <ComboField
                    label="Câmera"
                    listId="camera-list"
                    onChange={(value) => update('camera', value)}
                    placeholder="Digite a câmera"
                    suggestions={cameraOptions}
                    value={form.camera}
                  />
                  <SelectField label="Direção" value={form.direction} options={annotationOptions.direction} onChange={(value) => update('direction', value)} />
                  <label className="annotation-field">
                    <span>Resolução</span>
                    <select value={form.resolution} onChange={(event) => update('resolution', event.target.value)}>
                      <option value="">Selecione</option>
                      {Object.entries(codes.resolution ?? {}).map(([code, name]) => <option key={code} value={code}>{code} — {name}</option>)}
                    </select>
                  </label>
                  <SelectField label="Ângulo" value={form.capture_angle} options={annotationOptions.capture_angle} onChange={(value) => update('capture_angle', value)} />
                  <SelectField label="Período" value={form.day_time} options={annotationOptions.day_time} onChange={(value) => update('day_time', value)} />
                  <SelectField label="Alcance" value={form.image_range} options={annotationOptions.image_range} onChange={(value) => update('image_range', value)} />
                  <SelectField label="Clima" value={form.weather} options={annotationOptions.weather} onChange={(value) => update('weather', value)} />
                  <label className="annotation-field">
                    <span>Data e hora da captura</span>
                    <input type="datetime-local" value={form.captured_at} onChange={(event) => update('captured_at', event.target.value)} />
                  </label>
                </div>

                <label className="annotation-field annotation-condition">
                  <span>Condição</span>
                  <select value={form.conditions[0] ?? '01'} onChange={(event) => update('conditions', [event.target.value])}>
                    {annotationOptions.condition.map(([code, name]) => <option key={code} value={code}>{code} — {name}</option>)}
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
                      <SelectField label="Cor" value={vehicle.color} options={annotationOptions.color} onChange={(value) => annotator.updateVehicle(index, { color: value })} />
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

                <button className="button button--primary annotation-save" disabled={annotator.saving} onClick={annotator.save} type="button">
                  {annotator.saving ? 'Salvando...' : 'Salvar anotação'}
                </button>
              </div>
            </div>
          ) : (
            <div className="annotation-empty">Este pacote não possui imagens.</div>
          )}
        </main>
      </div>
    </section>
  )
}
