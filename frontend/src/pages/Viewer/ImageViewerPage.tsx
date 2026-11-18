import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ImageWithBoxes } from '../../components/ImageWithBoxes/ImageWithBoxes'
import { describeBoxSets, useImageBoxes } from '../../hooks/useImageBoxes'
import { getAnnotationProfile, getImage, getPackage, listPackageImages, reopenImage } from '../../services/api'
import type { AnnotationProfile, ImageDetail } from '../../types/annotation'
import { displayValue, fieldLabel } from '../../utils/annotationForm'
import './Viewer.css'

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
  const [profile, setProfile] = useState<AnnotationProfile | null>(null)
  const [total, setTotal] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [confirmEdit, setConfirmEdit] = useState(false)
  const [reopening, setReopening] = useState(false)
  const boxSets = useImageBoxes(image?.sha1)

  // O perfil do pacote: os nomes dos campos e das opções.
  useEffect(() => {
    let active = true
    getPackage(packageName)
      .then((summary) => (summary ? getAnnotationProfile(summary.profile) : null))
      .then((p) => { if (active) setProfile(p) })
      .catch(() => { /* sem o perfil, mostra as chaves e os valores crus */ })
    return () => { active = false }
  }, [packageName])

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
              {(profile?.fields ?? Object.keys(image.fields).map((key) => ({ key, type: 'text' as const }))).map((field) => (
                <Field key={field.key} label={fieldLabel(field)} value={displayValue(field, image.fields[field.key])} />
              ))}
            </div>

            {(profile?.objects ?? []).filter((type) => !type.box).map((type) => {
              const objects = image.objects.filter((object) => object.type === type.key)
              return (
                <div key={type.key}>
                  <h3>{fieldLabel(type)}</h3>
                  {objects.length ? objects.map((object, i) => {
                    const [first, ...rest] = type.fields
                    return (
                      <div className="viewer-vehicle" key={i}>
                        <strong>{first ? `${displayValue(first, object.fields[first.key]).replace('Não informado', `${fieldLabel(first)} não informado(a)`)}` : `${fieldLabel(type)} ${i + 1}`}</strong>
                        <span>{rest.map((field) => displayValue(field, object.fields[field.key])).join(' · ')}</span>
                      </div>
                    )
                  }) : <p className="viewer-empty">Nenhum(a).</p>}
                </div>
              )
            })}

            {image.file_name && (
              <p className="viewer-filename" title={image.pattern_version ? 'Nome v2 (usado no export; no disco o arquivo é o sha1)' : 'Nome do arquivo'}>{image.file_name}</p>
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
                A anotação atual fica como rascunho até ser verificada de novo, e as
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
