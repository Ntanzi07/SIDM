import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ImageWithBoxes } from '../../components/ImageWithBoxes/ImageWithBoxes'
import { annotationOptions } from '../../data/annotationOptions'
import { describeBoxSets, useImageBoxes } from '../../hooks/useImageBoxes'
import { getImage, listPackageImages, reopenImage } from '../../services/api'
import type { ImageDetail } from '../../types/annotation'
import './Viewer.css'

type Options = readonly (readonly [string, string])[]

// Valor da API -> rótulo em português ("rear" -> "Traseira").
function labelOf(options: Options, value: string | undefined): string {
  if (!value) return 'Não informado'
  return options.find(([option]) => option === value)?.[1] ?? value
}

function formatDate(iso: string | undefined): string {
  if (!iso) return 'Não informado'
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString('pt-BR')
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className={value === 'Não informado' ? 'viewer-field viewer-field--unknown' : 'viewer-field'}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

// Imagens VERIFICADAS de um pacote, só para ver/conferir. Duas formas:
//   /pacotes/:id/verificadas?i=3       navega por todas (anterior/próxima)
//   /pacotes/:id/verificadas?sha1=...  uma imagem só (vinda da lista do pacote)
// "Editar" devolve a imagem para pendente e abre na tela de anotação.
export function ImageViewerPage() {
  const navigate = useNavigate()
  const { id } = useParams()
  const packageName = decodeURIComponent(id ?? '')
  const [searchParams, setSearchParams] = useSearchParams()
  const shaParam = searchParams.get('sha1')
  const index = Math.max(0, Number.parseInt(searchParams.get('i') ?? '0', 10) || 0)

  const [image, setImage] = useState<ImageDetail | null>(null)
  const [total, setTotal] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [confirmEdit, setConfirmEdit] = useState(false)
  const [reopening, setReopening] = useState(false)
  const boxSets = useImageBoxes(image?.sha1)

  useEffect(() => {
    let active = true
    const load = async () => {
      if (shaParam) {
        return { detail: await getImage(shaParam), count: null }
      }
      const page = await listPackageImages(packageName, 1, index, 'verified')
      const first = page.items[0]
      return { detail: first ? await getImage(first.sha1) : null, count: page.total }
    }
    load()
      .then(({ detail, count }) => {
        if (!active) return
        setImage(detail)
        setTotal(count)
        setError('')
      })
      .catch((requestError: unknown) => { if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível abrir a imagem.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [index, packageName, shaParam])

  function go(nextIndex: number) {
    setLoading(true)
    setSearchParams({ i: String(nextIndex) })
  }

  async function handleEdit() {
    if (!image) return
    setReopening(true)
    setError('')
    try {
      await reopenImage(image.sha1)
      navigate(`/pacotes/${encodeURIComponent(packageName)}/anotacao?imagem=${image.sha1}`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível reabrir a imagem.')
      setReopening(false)
      setConfirmEdit(false)
    }
  }

  const back = () => navigate(`/pacotes/${encodeURIComponent(packageName)}`)

  return (
    <section className="viewer-page">
      <div className="viewer-header">
        <button className="back-button" onClick={back} type="button">← Voltar para o pacote</button>
        <div className="viewer-header__title">
          <h1>{packageName}</h1>
          <p>
            {shaParam
              ? 'Imagem verificada'
              : total !== null && total > 0
                ? `Verificada ${(index + 1).toLocaleString('pt-BR')} de ${total.toLocaleString('pt-BR')}`
                : 'Imagens verificadas'}
          </p>
        </div>
        {!shaParam && (
          <div className="viewer-nav">
            <button className="button button--secondary" disabled={loading || index === 0} onClick={() => go(index - 1)} type="button">← Anterior</button>
            <button className="button button--secondary" disabled={loading || total === null || index >= total - 1} onClick={() => go(index + 1)} type="button">Próxima →</button>
          </div>
        )}
      </div>

      {error && <p className="form-error">{error}</p>}
      {loading && !image && <p className="viewer-empty">Carregando...</p>}
      {!loading && !image && !error && <p className="viewer-empty">Nenhuma imagem verificada neste pacote ainda.</p>}

      {image && (
        <div className="viewer-layout">
          <div className="viewer-image">
            <ImageWithBoxes alt={image.original_name} sets={boxSets} sha1={image.sha1} show />
            <p className="viewer-image__meta">
              {image.original_name}
              {boxSets.length > 0 && ` · ${describeBoxSets(boxSets)}`}
            </p>
          </div>

          <div className="viewer-card">
            <div className="viewer-card__heading">
              <div>
                <h2>Anotação</h2>
                <span className={image.status === 'verified' ? 'viewer-badge' : 'viewer-badge viewer-badge--pending'}>
                  {image.status === 'verified' ? 'Verificada' : 'Pendente'}
                </span>
              </div>
              {image.status === 'verified' ? (
                <button className="button button--primary" disabled={reopening} onClick={() => setConfirmEdit(true)} type="button">Editar</button>
              ) : (
                <button className="button button--primary" onClick={() => navigate(`/pacotes/${encodeURIComponent(packageName)}/anotacao?imagem=${image.sha1}`)} type="button">Anotar</button>
              )}
            </div>

            <div className="viewer-fields">
              <Field label="Equipamento" value={image.equipment || 'Não informado'} />
              <Field label="Câmera" value={image.camera || 'Não informado'} />
              <Field label="Data e hora" value={formatDate(image.captured_at)} />
              <Field label="Direção" value={labelOf(annotationOptions.direction, image.direction)} />
              <Field label="Resolução" value={image.resolution || 'Não informado'} />
              <Field label="Ângulo" value={labelOf(annotationOptions.capture_angle, image.capture_angle)} />
              <Field label="Período" value={labelOf(annotationOptions.day_time, image.day_time)} />
              <Field label="Alcance" value={labelOf(annotationOptions.image_range, image.image_range)} />
              <Field label="Clima" value={labelOf(annotationOptions.weather, image.weather)} />
              <Field
                label="Condições"
                value={image.conditions?.length ? image.conditions.map((c) => labelOf(annotationOptions.condition, c)).join(', ') : 'Não informado'}
              />
            </div>

            <h3>Veículos</h3>
            {image.vehicles?.length ? image.vehicles.map((vehicle, i) => (
              <div className="viewer-vehicle" key={i}>
                <strong>{vehicle.plate || 'Placa não informada'}</strong>
                <span>
                  {[
                    labelOf(annotationOptions.type, vehicle.type),
                    labelOf(annotationOptions.plate_type, vehicle.plate_type),
                    labelOf(annotationOptions.plate_category, vehicle.plate_category),
                    labelOf(annotationOptions.color, vehicle.color),
                  ].join(' · ')}
                </span>
              </div>
            )) : <p className="viewer-empty">Nenhum veículo.</p>}

            {image.file_name && (
              <p className="viewer-filename" title="Nome do arquivo no padrão v2">{image.file_name}</p>
            )}
          </div>
        </div>
      )}

      {confirmEdit && image && (
        <div className="modal-backdrop" onClick={() => setConfirmEdit(false)} role="dialog" aria-modal="true">
          <div className="modal-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header">
              <h2>Editar imagem verificada</h2>
            </div>
            <div className="modal-body">
              <p>
                A imagem volta para <strong>pendente</strong> e abre na tela de anotação, reservada para você.
                O arquivo perde o nome v2 até ser verificada de novo. A anotação atual fica como rascunho e as
                caixas do CVAT não mudam.
              </p>
            </div>
            <div className="modal-actions">
              <button className="button button--secondary" disabled={reopening} onClick={() => setConfirmEdit(false)} type="button">Cancelar</button>
              <button className="button button--primary" disabled={reopening} onClick={() => void handleEdit()} type="button">
                {reopening ? 'Reabrindo...' : 'Voltar para pendente e editar'}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}
