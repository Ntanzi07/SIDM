import { useState } from 'react'
import { fixFromImage, type FromImageReport } from '../../services/api'
import { confirmDialog } from '../../services/confirm'
import { notify } from '../../services/toast'
import { SettingRow, SettingsGroup } from './SettingsUI'

const fmt = (n: number) => n.toLocaleString('pt-BR')

// Manutenção: a resolução (e outro campo que vem da imagem) das imagens
// gravadas antes da regra, quando era escolhida à mão. Primeiro o ensaio (só
// conta e mostra exemplos), depois corrigir (pede confirmação: muda o nome v2
// das verificadas).
export function FromImageFix() {
  const [report, setReport] = useState<FromImageReport | null>(null)
  const [busy, setBusy] = useState<'' | 'check' | 'fix'>('')

  async function check() {
    setBusy('check')
    try {
      setReport(await fixFromImage(true))
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível conferir.', 'error')
    } finally {
      setBusy('')
    }
  }

  async function fix() {
    if (!report || report.changed === 0) return
    const ok = await confirmDialog({
      title: `Corrigir ${fmt(report.changed)} ${report.changed === 1 ? 'imagem' : 'imagens'}?`,
      message: `A resolução passa a ser a do arquivo${report.verified ? `; em ${fmt(report.verified)} verificada(s) o nome v2 muda junto` : ''}. Cada mudança fica no histórico da imagem. Faça um backup do banco antes (mongodump, ver contexto/convencoes.md).`,
      confirmLabel: 'Corrigir',
      tone: 'danger',
    })
    if (!ok) return
    setBusy('fix')
    try {
      const done = await fixFromImage(false)
      notify(`${fmt(done.changed)} ${done.changed === 1 ? 'imagem corrigida' : 'imagens corrigidas'}${done.busy ? `; ${fmt(done.busy)} em anotação ficaram para depois` : ''}.`, 'success')
      setReport(await fixFromImage(true))
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível corrigir.', 'error')
    } finally {
      setBusy('')
    }
  }

  const summary = !report
    ? 'Para as imagens de antes de a resolução vir do arquivo, quando era escolhida à mão. Ensaie para ver quantas mudam.'
    : report.changed === 0
      ? `Tudo certo: as ${fmt(report.checked)} imagens já estão com a resolução do arquivo.`
      : `${fmt(report.changed)} de ${fmt(report.checked)} com a resolução diferente do arquivo${report.verified ? ` (${fmt(report.verified)} verificadas: o nome v2 muda)` : ''}${report.cleared ? `; em ${fmt(report.cleared)} o tamanho não está na lista e o campo fica vazio` : ''}.`

  return (
    <SettingsGroup description="Ações que mexem em muitas imagens de uma vez. Ensaie antes: o ensaio não muda nada." title="Manutenção">
      <SettingRow help={summary} label="Resolução pelo arquivo da imagem" stack={Boolean(report && (report.examples.length || report.errors.length || report.busy))}>
        <div className="fix-row">
          {report && (report.examples.length > 0 || report.busy > 0 || report.errors.length > 0) && (
            <ul className="fix-row__examples">
              {report.examples.map((item) => <li key={item}>{item}</li>)}
              {report.changed > report.examples.length && <li className="fix-row__more">e mais {fmt(report.changed - report.examples.length)}</li>}
              {report.busy > 0 && <li className="fix-row__more">{fmt(report.busy)} em anotação agora: ficam para depois</li>}
              {report.errors.slice(0, 3).map((item) => <li className="fix-row__error" key={item}>{item}</li>)}
            </ul>
          )}
          <div className="fix-row__actions">
            <button aria-busy={busy === 'check' || undefined} className="button button--secondary button--sm" disabled={busy !== ''} onClick={() => void check()} type="button">
              {busy === 'check' ? 'Conferindo…' : report ? 'Conferir de novo' : 'Ensaiar'}
            </button>
            {report && report.changed > 0 && (
              <button aria-busy={busy === 'fix' || undefined} className="button button--primary button--sm" disabled={busy !== ''} onClick={() => void fix()} type="button">
                {busy === 'fix' ? 'Corrigindo…' : `Corrigir ${fmt(report.changed)}`}
              </button>
            )}
          </div>
        </div>
      </SettingRow>
    </SettingsGroup>
  )
}
