import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { BoxImportModal } from './BoxImportModal'
import { cvatZipUrl, describeLinks, getPackageBoxes, linkPackageBoxes, type PackageBoxes } from '../../services/api'

const fmt = (n: number) => n.toLocaleString('pt-BR')
const pct = (n: number, of: number) => (of > 0 ? `${Math.min(100, Math.round((n / of) * 100))}%` : '0%')

// As caixas do pacote: em que tarefas ele tem imagens rotuladas e quanto do
// pacote cada uma cobre. Daqui sai o zip das imagens para rotular, e o
// import das caixas abre num pop-up (sem trocar de tela).
export function BoxesSection({ packageName, total }: { packageName: string; total: number }) {
  const [summary, setSummary] = useState<PackageBoxes | null>(null)
  const [importing, setImporting] = useState(false)
  const [message, setMessage] = useState('')
  const [reload, setReload] = useState(0) // muda depois de um import: o resumo é lido de novo
  const [linking, setLinking] = useState(false)

  // O import já liga as caixas aos objetos; isto refaz no pacote todo (caixas
  // importadas antes, ou depois de mudar o perfil).
  async function link() {
    setLinking(true)
    setMessage('')
    try {
      const result = await linkPackageBoxes(packageName)
      setMessage(describeLinks(result) || 'Nada a ligar: as caixas já estão ligadas aos objetos (ou o perfil não diz que label é qual objeto).')
    } catch (error: unknown) {
      setMessage(error instanceof Error ? error.message : 'Não foi possível ligar as caixas.')
    } finally {
      setLinking(false)
    }
  }

  useEffect(() => {
    let active = true
    getPackageBoxes(packageName)
      .then((result) => { if (active) setSummary(result) })
      .catch(() => { /* o resumo é só informativo */ })
    return () => { active = false }
  }, [packageName, reload])

  const tasks = summary?.tasks ?? []

  return (
    <section className="side-section">
      <div className="side-section__head">
        <h2>Caixas</h2>
        <span>{tasks.length} tarefa(s)</span>
      </div>
      {summary && tasks.length === 0 && (
        <p className="side-section__hint">Nenhuma imagem deste pacote tem caixas ainda. Baixe as imagens, rotule e importe as caixas.</p>
      )}
      {tasks.length > 0 && (
        <ul className="task-list">
          {tasks.map((item) => (
            <li key={item.task}>
              <Link className="task-row" title="Abrir a tarefa" to={`/tarefas/${encodeURIComponent(item.task)}`}>
                <span className="task-row__top">
                  <strong>{item.task}</strong>
                  <small>{pct(item.images, total)}</small>
                </span>
                <span className="task-row__bar"><span style={{ width: pct(item.images, total) }} /></span>
                <small>{fmt(item.images)} de {fmt(total)} imagens · {fmt(item.boxes)} caixas</small>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {message && <p className="side-section__message">{message}</p>}
      <div className="side-section__actions">
        <button className="button button--secondary button--sm" onClick={() => { setMessage(''); setImporting(true) }} type="button">Importar caixas</button>
        <a className="button button--secondary button--sm" download href={cvatZipUrl(packageName)}>Baixar imagens para rotular</a>
        {tasks.length > 0 && (
          <button
            className="button button--secondary button--sm"
            disabled={linking}
            onClick={() => void link()}
            title="Liga as caixas aos objetos (ex.: a caixa motorcycle vira um veículo), como o import já faz"
            type="button"
          >
            {linking ? 'Ligando...' : 'Ligar caixas aos objetos'}
          </button>
        )}
      </div>
      {importing && (
        <BoxImportModal
          defaultTask={tasks.length === 1 ? tasks[0].task : undefined}
          onApplied={(text) => { setImporting(false); setMessage(text); setReload((n) => n + 1) }}
          onClose={() => setImporting(false)}
        />
      )}
    </section>
  )
}
