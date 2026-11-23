import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { listDatasets, type Dataset } from '../../services/api'
import { fmt } from '../../utils/stats'
import { formatWhen } from './format'
import './Versions.css'

// Versões: os datasets (pesquisas salvas) e a versão mais nova de cada um.
// Um dataset nasce na Pesquisa ("Salvar como dataset").
export function DatasetsPage() {
  const navigate = useNavigate()
  const [datasets, setDatasets] = useState<Dataset[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    listDatasets()
      .then((items) => { if (active) setDatasets(items) })
      .catch((requestError: unknown) => {
        if (!active) return
        setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os datasets.')
        setDatasets([])
      })
    return () => { active = false }
  }, [])

  return (
    <section className="versions-page">
      <div className="page-heading">
        <div>
          <h1>Versões</h1>
          <p>Cada dataset é uma pesquisa salva. Uma versão congela as imagens verificadas e a anotação delas: pronta para treinar, e nunca muda.</p>
        </div>
        <button className="button button--primary" onClick={() => navigate('/pesquisa')} type="button">
          Novo dataset pela Pesquisa
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}

      {datasets === null && <p className="versions-hint">Carregando...</p>}
      {datasets?.length === 0 && !error && (
        <div className="versions-empty">
          <p>Nenhum dataset ainda.</p>
          <p className="versions-hint">Na Pesquisa, escolha o perfil, os pacotes, os filtros e as tarefas, e use “Salvar como dataset”.</p>
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
                </div>
              ) : (
                <p className="dataset-card__none">Nenhuma versão ainda</p>
              )}
            </Link>
          ))}
        </div>
      )}
    </section>
  )
}
