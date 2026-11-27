import { useEffect, useState } from 'react'
import type { ImportPhase, ImportStep } from '../../services/api'

// O andamento do import (ensaio ou aplicar): a etapa, a barra com "N de M"
// e o tempo. As etapas são as do servidor (imports/progress.go), em ordem.

type Props = {
  step: ImportStep | null // null: ainda mandando o arquivo / esperando o servidor
  startedAt: number
  dryRun: boolean
  source: 'csv' | 'ini'
  stoppable?: boolean // há um botão de parar o ensaio
}

const fmt = (n: number) => n.toLocaleString('pt-BR')

function elapsed(ms: number): string {
  const s = Math.floor(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

export function ImportProgress({ step, startedAt, dryRun, source, stoppable = false }: Props) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const phases: Array<[ImportPhase | 'enviando', string]> = [
    ['enviando', source === 'csv' ? 'Enviando o CSV' : 'Começando'],
    ['lendo', source === 'csv' ? 'Lendo o CSV e achando as imagens do pacote' : 'Lendo o INI das imagens pendentes'],
    ['conferindo', 'Conferindo o que muda em cada imagem'],
    ['cadastros', 'Conferindo equipamentos e câmeras novos'],
    ...(dryRun ? [] : [['gravando', 'Gravando os rascunhos'] as [ImportPhase, string]]),
  ]
  const current = step?.phase ?? 'enviando'
  const index = phases.findIndex(([phase]) => phase === current)
  const label = phases[index]?.[1] ?? 'Importando'
  const counted = step !== null && step.total > 0
  const percent = counted ? Math.min(100, Math.round((step.done / step.total) * 100)) : 0
  const unit = current === 'gravando' ? 'imagens' : source === 'csv' ? 'linhas' : 'imagens'

  return (
    <div className="import-progress">
      <div className="import-progress__head">
        <strong>{dryRun ? 'Ensaiando' : 'Aplicando'}: etapa {index + 1} de {phases.length}</strong>
        <span className="import-progress__time">{elapsed(now - startedAt)}</span>
      </div>
      <p aria-live="polite" className="import-progress__label" role="status">
        {label}
        {counted && <> · {fmt(step.done)} de {fmt(step.total)} {unit}</>}
      </p>
      <div
        aria-label={label}
        aria-valuemax={counted ? 100 : undefined}
        aria-valuemin={counted ? 0 : undefined}
        aria-valuenow={counted ? percent : undefined}
        className={counted ? 'import-progress__bar' : 'import-progress__bar import-progress__bar--busy'}
        role="progressbar"
      >
        <span style={counted ? { width: `${percent}%` } : undefined} />
      </div>
      <ol className="import-progress__steps">
        {phases.map(([phase, text], i) => (
          <li className={i < index ? 'is-done' : i === index ? 'is-now' : ''} key={phase}>{text}</li>
        ))}
      </ol>
      <p className="import-progress__hint">
        {dryRun
          ? stoppable ? 'O ensaio não muda nada: dá para parar a qualquer momento.' : 'O ensaio não muda nada nas imagens.'
          : 'Não feche esta janela até terminar: parar no meio deixa só parte das imagens gravada.'}
      </p>
    </div>
  )
}
