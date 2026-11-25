import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { describeLinks, importTaskBoxes, listTasks, type BoxImportReport } from '../../services/api'
import { BoxImportSummary } from '../Tasks/BoxImportReport'
import { useToastState } from '../../hooks/useToast'

const fmt = (n: number) => n.toLocaleString('pt-BR')
const sizeText = (bytes: number) => (bytes < 1024 * 1024
  ? `${Math.max(1, Math.round(bytes / 1024)).toLocaleString('pt-BR')} KB`
  : `${(bytes / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} MB`)

// Os formatos de arquivo de caixas que o import entende. Hoje só o COCO 1.0
// (o export do CVAT); quando houver outro, a caixa vira uma escolha.
const FORMATS = [{ value: 'coco', label: 'COCO 1.0', accept: '.zip,.json,application/zip,application/json', hint: 'o zip do export (ou só o .json)' }]

// Importar caixas sem sair do pacote: a tarefa, o formato, o arquivo e o
// "sem objetos"; primeiro o ensaio, depois gravar. As imagens do arquivo são
// procuradas em todos os pacotes (o ensaio mostra quantas por pacote).
export function BoxImportModal({ defaultTask, fixedTask = false, onClose, onApplied }: {
  defaultTask?: string
  fixedTask?: boolean // aberto de dentro de uma tarefa: não troca
  onClose: () => void
  onApplied: (message: string) => void
}) {
  const [tasks, setTasks] = useState<string[] | null>(null)
  const [task, setTask] = useState(defaultTask ?? '')
  const [format] = useState(FORMATS[0])
  const [file, setFile] = useState<File | null>(null)
  const [negatives, setNegatives] = useState(false)
  const [report, setReport] = useState<BoxImportReport | null>(null)
  const [, setError] = useToastState('error')
  const [busy, setBusy] = useState<'testing' | 'applying' | ''>('')

  useEffect(() => {
    let active = true
    listTasks()
      .then((items) => {
        if (!active) return
        const names = items.map((item) => item.name).sort()
        setTasks(names)
        // uma tarefa só: já vem escolhida
        if (names.length === 1) setTask((current) => current || names[0])
      })
      .catch((requestError: unknown) => {
        if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as tarefas.')
      })
    return () => { active = false }
  }, [setError])

  const close = () => { if (!busy) onClose() }

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  function reset() {
    setReport(null)
    setError('')
  }

  async function run(dryRun: boolean) {
    if (!task || !file) return
    setBusy(dryRun ? 'testing' : 'applying')
    setError('')
    try {
      const result = await importTaskBoxes(task, file, dryRun, negatives)
      if (dryRun) setReport(result)
      else onApplied(`${fmt(result.applied_boxes)} caixa(s) gravada(s) em ${fmt(result.applied)} imagem(ns) na tarefa ${task}. ${describeLinks(result.links)}`.trim())
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível importar as caixas.')
    } finally {
      setBusy('')
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    void run(true)
  }

  // no body: um ancestral com transform (as animações) prenderia o fixed
  return createPortal(
    <div aria-modal="true" className="modal-backdrop" onClick={close} role="dialog">
      <div className="modal-dialog import-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h2>Importar caixas</h2>
          <button aria-label="Fechar" className="modal-close" disabled={busy !== ''} onClick={close} type="button">✕</button>
        </div>
        <form onSubmit={submit}>
          <div className="modal-body">
            <p className="import-modal__intro">
              As caixas rotuladas voltam para as imagens do SIDM. As imagens do arquivo são procuradas em todos os pacotes; o ensaio mostra quantas são de cada um.
            </p>

            <div className="import-modal__row">
              <label className="import-field">
                <span>Tarefa</span>
                <select disabled={busy !== '' || tasks === null || fixedTask} onChange={(event) => { setTask(event.target.value); reset() }} value={task}>
                  <option value="">{tasks === null ? 'Carregando...' : 'Escolha a tarefa'}</option>
                  {tasks?.map((name) => <option key={name} value={name}>{name}</option>)}
                </select>
              </label>
              <label className="import-field" title="Por enquanto, só o COCO 1.0">
                <span>Formato</span>
                <select disabled value={format.value}>
                  {FORMATS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
                </select>
              </label>
            </div>
            {tasks?.length === 0 && (
              <p className="form-hint">Nenhuma tarefa ainda. <Link to="/tarefas">Crie uma em Tarefas</Link> (o nome e as labels).</p>
            )}

            <label className="import-modal__file">
              <span className="import-modal__file-icon">⇪</span>
              <span>
                <strong>{file?.name ?? `Selecionar o arquivo ${format.label}`}</strong>
                <small>{file ? `${sizeText(file.size)} · clique para trocar` : format.hint}</small>
              </span>
              <input accept={format.accept} disabled={busy !== ''} onChange={(event) => { setFile(event.target.files?.[0] ?? null); reset() }} type="file" />
            </label>

            <label className="import-option import-option--top">
              <input checked={negatives} disabled={busy !== ''} onChange={(event) => { setNegatives(event.target.checked); reset() }} type="checkbox" />
              <span>
                Imagens sem nenhuma caixa entram como “sem objetos”
                <small>Marque só se a tarefa estiver toda rotulada: o export traz todas as imagens, inclusive as que ninguém olhou ainda. Desmarcado, elas são ignoradas.</small>
              </span>
            </label>


            {report && (
              <div className="import-report">
                <h2>Ensaio: nada foi gravado ainda</h2>
                <BoxImportSummary report={report} />
              </div>
            )}
          </div>
          <div className="modal-actions">
            <button className="button button--secondary" disabled={busy !== ''} onClick={close} type="button">Cancelar</button>
            {report ? (
              <button className="button button--primary" disabled={busy !== '' || report.will_apply === 0} onClick={() => void run(false)} type="button">
                {busy === 'applying' ? 'Gravando...' : `Gravar caixas em ${fmt(report.will_apply)} imagem(ns)`}
              </button>
            ) : (
              <button className="button button--primary" disabled={busy !== '' || !task || !file} type="submit">
                {busy === 'testing' ? 'Ensaiando...' : 'Ensaiar'}
              </button>
            )}
          </div>
        </form>
      </div>
    </div>,
    document.body,
  )
}
