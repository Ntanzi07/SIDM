import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { deleteImage, imageFileUrl, listPackageImages, type ImageSummary } from '../../services/api'
import { useToastState } from '../../hooks/useToast'
import { confirmDialog } from '../../services/confirm'

const PAGE = 25
const fmt = (n: number) => n.toLocaleString('pt-BR')

type Status = '' | ImageSummary['status']

const FILTERS: Array<{ value: Status; label: string }> = [
  { value: '', label: 'Todas' },
  { value: 'pending', label: 'Pendentes' },
  { value: 'verified', label: 'Verificadas' },
]

// As imagens do pacote em rolagem infinita: busca 25, e quando o fim da lista
// chega perto da tela (o "sentinela", observado pelo IntersectionObserver),
// busca as próximas 25. Nada de buscar milhares de uma vez.
export function PackageImages({ packageName, onRemoved }: { packageName: string; onRemoved: () => void }) {
  const [status, setStatus] = useState<Status>('')
  return (
    <div className="package-images">
      <div className="package-images__filters" role="tablist" aria-label="Status das imagens">
        {FILTERS.map((item) => (
          <button
            aria-selected={status === item.value}
            className={status === item.value ? 'images-filter images-filter--on' : 'images-filter'}
            key={item.value || 'all'}
            onClick={() => setStatus(item.value)}
            role="tab"
            type="button"
          >
            {item.label}
          </button>
        ))}
      </div>
      {/* a key recomeça a lista quando o filtro muda */}
      <ImageFeed key={status} onRemoved={onRemoved} packageName={packageName} status={status} />
    </div>
  )
}

function ImageFeed({ packageName, status, onRemoved }: { packageName: string; status: Status; onRemoved: () => void }) {
  const navigate = useNavigate()
  const [items, setItems] = useState<ImageSummary[]>([])
  const [total, setTotal] = useState(-1) // -1 = ainda não chegou a primeira página
  const [error, setError] = useToastState('error')
  const [removing, setRemoving] = useState('')
  const loading = useRef(false)
  const sentinel = useRef<HTMLDivElement | null>(null)

  const loadMore = useCallback(async (skip: number) => {
    if (loading.current) return
    loading.current = true
    try {
      const page = await listPackageImages(packageName, PAGE, skip, status || undefined)
      setItems((current) => [...current, ...page.items])
      setTotal(page.total)
      setError('')
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível buscar as imagens.')
    } finally {
      loading.current = false
    }
  }, [packageName, status, setError])

  // a primeira página
  useEffect(() => {
    let active = true
    loading.current = true
    listPackageImages(packageName, PAGE, 0, status || undefined)
      .then((page) => {
        if (!active) return
        setItems(page.items)
        setTotal(page.total)
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível buscar as imagens.')
      })
      .finally(() => { loading.current = false })
    return () => { active = false }
  }, [packageName, status, setError])

  // o sentinela no fim da lista: apareceu (com 400px de folga) = busca mais
  const hasMore = total < 0 || items.length < total
  useEffect(() => {
    const node = sentinel.current
    if (!node || !hasMore || total < 0 || error) return
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void loadMore(items.length)
    }, { rootMargin: '400px 0px' })
    observer.observe(node)
    return () => observer.disconnect()
  }, [hasMore, items.length, loadMore, total, error])

  function open(image: ImageSummary) {
    const pkg = encodeURIComponent(packageName)
    navigate(image.status === 'verified' ? `/pacotes/${pkg}/verificadas?sha1=${image.sha1}` : `/pacotes/${pkg}/anotacao?imagem=${image.sha1}`)
  }

  async function remove(image: ImageSummary) {
    if (!(await confirmDialog({
      title: 'Remover imagem?',
      message: `"${image.original_name}" e a anotação dela serão removidas permanentemente.`,
      confirmLabel: 'Remover',
      tone: 'danger',
    }))) return
    setRemoving(image.sha1)
    try {
      await deleteImage(image.sha1)
      setItems((current) => current.filter((item) => item.sha1 !== image.sha1))
      setTotal((current) => current - 1)
      onRemoved()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível remover a imagem.')
    } finally {
      setRemoving('')
    }
  }

  return (
    <>
      {total === 0 && <p className="package-images__empty">{status ? 'Nenhuma imagem com este status.' : 'Este pacote ainda não tem imagens.'}</p>}
      {items.length > 0 && (
        <ul className="image-feed">
          {items.map((image, index) => (
            <li className="image-feed__item" key={image.sha1} style={{ animationDelay: `${(index % PAGE) * 18}ms` }}>
              <button className="image-feed__open" onClick={() => open(image)} title={image.status === 'verified' ? 'Ver a anotação' : 'Abrir na anotação'} type="button">
                <img alt="" decoding="async" loading="lazy" src={imageFileUrl(image.sha1)} />
                <span className="image-feed__name">{image.original_name}</span>
              </button>
              <span className={image.status === 'verified' ? 'image-status image-status--verified' : 'image-status'}>
                <i className={image.status === 'verified' ? 'status-dot status-dot--verified' : 'status-dot status-dot--pending'} />
                {image.status === 'verified' ? 'Verificada' : 'Pendente'}
              </span>
              <button
                aria-label={`Remover ${image.original_name}`}
                className="image-feed__remove"
                disabled={removing !== ''}
                onClick={() => void remove(image)}
                title="Remover a imagem"
                type="button"
              >
                ✕
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="image-feed__sentinel" ref={sentinel}>
        {total < 0 && 'Carregando...'}
        {total > 0 && hasMore && 'Carregando mais...'}
        {total > 0 && !hasMore && items.length > PAGE && `Fim · ${fmt(total)} imagem(ns)`}
      </div>
    </>
  )
}
