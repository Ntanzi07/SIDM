import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { cvatZipUrl, getPackageBoxes, importBoxes, type BoxImportReport, type PackageBoxes } from '../../services/api'

const fmt = (n: number) => n.toLocaleString('pt-BR')

function labelList(labels: Record<string, number>) {
  return Object.entries(labels)
    .sort(([, a], [, b]) => b - a)
    .map(([label, n]) => `${label} ${fmt(n)}`)
    .join(' · ')
}

// Caixas (bounding boxes) anotadas no CVAT: baixar as imagens para criar a
// task, e importar o export COCO 1.0 de volta (ensaio, depois aplicar).
export function BoxesSection({ packageName }: { packageName: string }) {
  const [summary, setSummary] = useState<PackageBoxes | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [report, setReport] = useState<BoxImportReport | null>(null)
  const [saving, setSaving] = useState<'' | 'testing' | 'applying'>('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  useEffect(() => {
    let active = true
    getPackageBoxes(packageName)
      .then((result) => { if (active) setSummary(result) })
      .catch(() => { /* o resumo é só informativo */ })
    return () => { active = false }
  }, [packageName])

  async function run(dryRun: boolean) {
    if (!file) return
    setSaving(dryRun ? 'testing' : 'applying')
    setError('')
    setMessage('')
    try {
      const result = await importBoxes(packageName, file, dryRun)
      if (dryRun) {
        setReport(result)
      } else {
        setReport(null)
        setFile(null)
        setMessage(`${fmt(result.applied_boxes)} caixa(s) gravada(s) em ${fmt(result.applied)} imagem(ns).`)
        setSummary(await getPackageBoxes(packageName))
      }
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível importar as caixas.')
    } finally {
      setSaving('')
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    void run(true)
  }

  const willApply = report ? report.matched - report.empty - report.size_mismatch.length : 0

  return (
    <form className="existing-import" onSubmit={handleSubmit}>
      <div className="detail-images__heading">
        <div>
          <h2>Caixas (CVAT)</h2>
          <p>
            {summary && summary.images > 0
              ? `${fmt(summary.images)} imagem(ns) com caixas · ${labelList(summary.labels)}`
              : 'As caixas são anotadas no CVAT. Baixe as imagens, crie a task lá e importe o export COCO 1.0 aqui.'}
          </p>
        </div>
        <a className="button button--secondary" download href={cvatZipUrl(packageName)}>
          Baixar imagens para o CVAT
        </a>
      </div>
      <div className="existing-import__controls">
        <label className="existing-import__file">
          <span>{file?.name ?? 'Selecione o export do CVAT (COCO 1.0: .zip ou .json)'}</span>
          <input
            accept=".zip,.json,application/zip,application/json"
            disabled={saving !== ''}
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null)
              setReport(null)
              setError('')
              setMessage('')
            }}
            type="file"
          />
        </label>
        <button className="button button--secondary" disabled={saving !== '' || !file} type="submit">
          {saving === 'testing' ? 'Ensaiando...' : 'Ensaiar import'}
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}
      {message && <p className="form-hint">{message}</p>}
      {report && (
        <div className="import-report">
          <strong>Ensaio das caixas</strong>
          <div className="import-report__stats">
            <span><strong>{fmt(report.file_images)}</strong> imagens no arquivo</span>
            <span><strong>{fmt(report.matched)}</strong> encontradas no pacote</span>
            <span><strong>{fmt(willApply)}</strong> receberão caixas</span>
            <span><strong>{fmt(report.not_found_count)}</strong> não encontradas</span>
          </div>
          {Object.keys(report.labels).length > 0 && (
            <p className="import-report__detail"><strong>Caixas por label:</strong> {labelList(report.labels)}</p>
          )}
          {report.replaced > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              {fmt(report.replaced)} imagem(ns) já têm caixas: serão substituídas.
            </p>
          )}
          {report.empty > 0 && (
            <p className="import-report__detail">{fmt(report.empty)} imagem(ns) sem nenhuma caixa no arquivo: não serão mexidas.</p>
          )}
          {report.not_found_count > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              <strong>Não estão neste pacote:</strong> {report.not_found.slice(0, 5).join(', ')}{report.not_found_count > 5 && ', …'}
              {' '}— crie a task do CVAT com o zip "Baixar imagens para o CVAT" para os nomes casarem.
            </p>
          )}
          {report.ambiguous.length > 0 && (
            <p className="import-report__detail import-report__detail--warning">
              <strong>Nome repetido no pacote:</strong> {report.ambiguous.slice(0, 5).join(', ')}
            </p>
          )}
          {report.size_mismatch.slice(0, 5).map((item) => (
            <p className="import-report__detail import-report__detail--warning" key={item.file_name}>
              {item.file_name}: tamanho no CVAT {item.cvat}, no SIDM {item.sidm} — pulada (as caixas ficariam fora do lugar).
            </p>
          ))}
          {report.warnings.slice(0, 5).map((warning) => (
            <p className="import-report__detail import-report__detail--warning" key={warning}>{warning}</p>
          ))}
          <button
            className="button button--primary"
            disabled={saving !== '' || willApply === 0}
            onClick={() => void run(false)}
            type="button"
          >
            {saving === 'applying' ? 'Gravando...' : `Gravar caixas em ${fmt(willApply)} imagem(ns)`}
          </button>
        </div>
      )}
    </form>
  )
}
