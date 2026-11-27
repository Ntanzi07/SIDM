import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { runImport, type ImportProfile, type ImportReport, type ImportStep } from '../../services/api'
import { ImportProgress } from './ImportProgress'
import { ImportWarnings } from './ImportWarnings'
import { forPackage, optionLabel } from '../../utils/importProfiles'
import { useToastState } from '../../hooks/useToast'

export type ImportSource = 'csv' | 'ini'

// Importar anotações para os rascunhos do pacote: de um CSV enviado agora, ou
// do INI que já está guardado em cada imagem. Primeiro um ensaio (nada muda),
// depois aplicar.
export function ImportModal({ packageName, packageProfile, source: initialSource, csvProfiles, iniProfiles, onClose, onApplied }: {
  packageName: string
  packageProfile: string
  source: ImportSource
  csvProfiles: ImportProfile[]
  iniProfiles: ImportProfile[]
  onClose: () => void
  onApplied: () => void
}) {
  const [source, setSource] = useState<ImportSource>(initialSource)
  const [profile, setProfile] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [overwrite, setOverwrite] = useState(true)
  const [report, setReport] = useState<ImportReport | null>(null)
  const [, setError] = useToastState('error')
  const [busy, setBusy] = useState<'testing' | 'applying' | ''>('')
  // o andamento (o servidor manda as etapas) e quando começou
  const [step, setStep] = useState<ImportStep | null>(null)
  const [startedAt, setStartedAt] = useState(0)
  const abort = useRef<AbortController | null>(null)

  const options = forPackage(source === 'csv' ? csvProfiles : iniProfiles, packageProfile)
  const ready = profile !== '' && (source === 'ini' || file !== null)

  function reset() {
    setReport(null)
    setError('')
  }

  async function run(dryRun: boolean) {
    if (!ready) return
    setBusy(dryRun ? 'testing' : 'applying')
    setError('')
    setStep(null)
    setStartedAt(Date.now())
    // só o ensaio pode parar (aplicar pela metade deixa parte gravada)
    abort.current = dryRun ? new AbortController() : null
    try {
      const result = await runImport(packageName, profile, source === 'csv' ? file : null, dryRun, overwrite, setStep, abort.current?.signal)
      if (dryRun) setReport(result)
      else onApplied()
    } catch (requestError: unknown) {
      if (requestError instanceof DOMException && requestError.name === 'AbortError') return // parou o ensaio
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível importar.')
    } finally {
      abort.current = null
      setBusy('')
      setStep(null)
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    void run(true)
  }

  const close = () => { if (!busy) onClose() }

  // Esc fecha (menos no meio de um pedido)
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  // no body: um ancestral com transform (as animações) prenderia o fixed
  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={close} role="dialog">
      <div className="modal-dialog import-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Importar anotações</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy !== ''} onClick={close} type="button">✕</button>
        </div>
        <form onSubmit={submit}>
          <div className="modal-body">
            {iniProfiles.length > 0 && (
              <div aria-label="Origem dos metadados" className="import-source" role="radiogroup">
                {(['csv', 'ini'] as const).map((item) => (
                  <button
                    aria-checked={source === item}
                    className={source === item ? 'import-source__option import-source__option--active' : 'import-source__option'}
                    disabled={busy !== ''}
                    key={item}
                    onClick={() => { setSource(item); setProfile(''); reset() }}
                    role="radio"
                    type="button"
                  >
                    {item === 'csv' ? 'Arquivo CSV' : 'INI das imagens'}
                  </button>
                ))}
              </div>
            )}
            <p className="import-modal__intro">
              {source === 'csv'
                ? 'Preenche os rascunhos das imagens deste pacote com as linhas de um CSV, usando um perfil de importação.'
                : 'Preenche os rascunhos das imagens pendentes com o INI que veio dentro de cada JPEG.'}
            </p>

            <label className="import-field">
              <span>{source === 'csv' ? 'Perfil CSV' : 'Perfil de INI'}</span>
              <select disabled={busy !== ''} onChange={(event) => { setProfile(event.target.value); reset() }} value={profile}>
                <option value="">Selecione um perfil</option>
                {options.map((item) => <option key={item.name} value={item.name}>{optionLabel(item, packageProfile)}</option>)}
              </select>
            </label>
            {options.length === 0 && <p className="form-hint">Nenhum perfil para este pacote. Crie um em Configurar.</p>}

            {source === 'csv' && (
              <label className="import-modal__file">
                <span className="import-modal__file-icon">⇪</span>
                <span>
                  <strong>{file?.name ?? 'Selecionar o arquivo CSV'}</strong>
                  <small>{file ? `${(file.size / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 0 })} KB · clique para trocar` : 'Uma linha por imagem'}</small>
                </span>
                <input accept=".csv,text/csv" disabled={busy !== ''} onChange={(event) => { setFile(event.target.files?.[0] ?? null); reset() }} type="file" />
              </label>
            )}

            <label className="import-option">
              <input checked={overwrite} disabled={busy !== ''} onChange={(event) => { setOverwrite(event.target.checked); reset() }} type="checkbox" />
              <span>Sobrescrever os rascunhos que já têm valor</span>
            </label>


            {busy !== '' && <ImportProgress dryRun={busy === 'testing'} source={source} startedAt={startedAt} step={step} stoppable />}

            {report && busy === '' && (
              <div className="import-report">
                <h2>Ensaio: nada foi alterado ainda</h2>
                <div className="import-report__stats">
                  <span><strong>{report.rows}</strong> {source === 'csv' ? 'linhas lidas' : 'imagens com INI'}</span>
                  <span><strong>{report.matched}</strong> encontradas</span>
                  <span><strong>{report.applied}</strong> alterações</span>
                  <span><strong>{report.not_found_count}</strong> não encontradas</span>
                </div>
                <ImportWarnings report={report} />
              </div>
            )}
          </div>
          <div className="modal-actions">
            {busy === 'testing'
              ? <button className="button button--secondary" onClick={() => abort.current?.abort()} type="button">Parar o ensaio</button>
              : <button className="button button--secondary" disabled={busy !== ''} onClick={close} type="button">Cancelar</button>}
            {report ? (
              <button aria-busy={busy === 'applying' || undefined} className="button button--primary" disabled={busy !== ''} onClick={() => void run(false)} type="button">
                {busy === 'applying' ? 'Aplicando…' : 'Aplicar importação'}
              </button>
            ) : (
              <button aria-busy={busy === 'testing' || undefined} className="button button--primary" disabled={busy !== '' || !ready} type="submit">
                {busy === 'testing' ? 'Ensaiando…' : 'Ensaiar'}
              </button>
            )}
          </div>
        </form>
      </div>
    </div>,
    document.body,
  )
}
