import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { listDatasets, listVersions, type Dataset } from '../../services/api'
import { Sparkline } from '../../components/Charts/Sparkline'
import { fmt } from '../../utils/stats'
import { formatWhen } from './format'
import { useToastState } from '../../hooks/useToast'
import './Versions.css'

// Os datasets (pesquisas salvas) de um perfil e a versão mais nova de cada
// um: a aba Versões do Dashboard. Um dataset nasce na aba Dados ("Salvar
// como dataset").
export function DatasetList({ profile }: { profile: string }) {
  const [datasets, setDatasets] = useState<Dataset[] | null>(null)
  const [error, setError] = useToastState('error')
  const [history, setHistory] = useState<Record<string, number[]>>({})

  useEffect(() => {
    let active = true
    listDatasets()
      .then((items) => {
        if (!active) return
        const mine = items.filter((item) => item.profile === profile)
        setDatasets(mine)
        // as imagens de cada versão, para a linha pequena do cartão
        for (const item of mine) {
          if (!item.latest) continue
          listVersions(item.name)
            .then((list) => {
              if (active) setHistory((current) => ({ ...current, [item.name]: list.map((v) => v.summary.images.total).reverse() }))
            })
            .catch(() => { /* só informativo */ })
        }
      })
      .catch((requestError: unknown) => {
        if (!active) return
        setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os datasets.')
        setDatasets([])
      })
    return () => { active = false }
  }, [profile, setError])

  return (
    <div className="versions-list">

      {datasets === null && <p className="versions-hint">Carregando...</p>}
      {datasets?.length === 0 && !error && (
        <div className="versions-empty">
          <p>Nenhum dataset ainda.</p>
          <p className="versions-hint">Na aba Dados, escolha os pacotes, os filtros e as tarefas, e use “Salvar como dataset”.</p>
        </div>
      )}

      {datasets && datasets.length > 0 && (
        <div className="dataset-grid">
          {datasets.map((dataset, index) => (
            <Link
              className="dataset-card"
              key={dataset.name}
              style={{ animationDelay: `${Math.min(index, 10) * 45}ms` }}
              to={`/versoes/${encodeURIComponent(dataset.name)}`}
            >
              <span className="eyebrow">{dataset.profile}</span>
              <h2>{dataset.name}</h2>
              {dataset.description && <p className="dataset-card__description">{dataset.description}</p>}
              {dataset.latest ? (
                <div className="dataset-card__latest">
                  <span className="version-badge">v{dataset.latest.number}</span>
                  <div>
                    <p>{dataset.latest.message}</p>
                    <small>
                      {dataset.latest.created_by} · {formatWhen(dataset.latest.created_at)} · {fmt(dataset.latest.images)} imagem(ns)
                    </small>
                  </div>
                  {(history[dataset.name]?.length ?? 0) > 1 && (
                    <span className="dataset-card__spark" title="Imagens por versão">
                      <Sparkline values={history[dataset.name]} width={90} height={30} />
                    </span>
                  )}
                </div>
              ) : (
                <p className="dataset-card__none">Nenhuma versão ainda</p>
              )}
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
