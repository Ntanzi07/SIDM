import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { cvatZipUrl, getPackageBoxes, type PackageBoxes } from '../../services/api'

const fmt = (n: number) => n.toLocaleString('pt-BR')

// Caixas (bounding boxes) do CVAT neste pacote: em que tarefas ele tem
// imagens anotadas, e o zip das imagens para criar a task no CVAT. O import
// das caixas é feito na página da tarefa (procura as imagens em todos os
// pacotes).
export function BoxesSection({ packageName }: { packageName: string }) {
  const [summary, setSummary] = useState<PackageBoxes | null>(null)

  useEffect(() => {
    let active = true
    getPackageBoxes(packageName)
      .then((result) => { if (active) setSummary(result) })
      .catch(() => { /* o resumo é só informativo */ })
    return () => { active = false }
  }, [packageName])

  const tasks = summary?.tasks ?? []

  return (
    <div className="existing-import">
      <div className="detail-images__heading">
        <div>
          <h2>Caixas (CVAT)</h2>
          <p>
            {tasks.length > 0
              ? 'Imagens deste pacote anotadas em cada tarefa:'
              : 'Nenhuma imagem deste pacote está numa tarefa ainda. Baixe as imagens, crie a task no CVAT e importe o export na página da tarefa.'}
          </p>
        </div>
        <a className="button button--secondary" download href={cvatZipUrl(packageName)}>
          Baixar imagens para o CVAT
        </a>
      </div>
      {tasks.length > 0 && (
        <div className="import-report__stats">
          {tasks.map((item) => (
            <span key={item.task}>
              <Link to={`/tarefas/${encodeURIComponent(item.task)}`}><strong>{item.task}</strong></Link>
              {' '}{fmt(item.images)} imagem(ns) · {fmt(item.boxes)} caixa(s)
            </span>
          ))}
        </div>
      )}
      <p className="form-hint">
        Para importar as caixas, abra a tarefa em <Link to="/tarefas">Tarefas</Link>.
      </p>
    </div>
  )
}
