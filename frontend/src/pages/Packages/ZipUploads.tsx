import type { ZipUpload } from '../../services/api'
import type { useZipUploads } from '../../hooks/useZipUploads'

const fmt = (n: number) => n.toLocaleString('pt-BR')
const mb = (n: number) => `${(n / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: n > 10 * 1024 * 1024 * 1024 ? 0 : 1 })} MB`
const pct = (n: number, of: number) => (of > 0 ? Math.min(100, Math.round((n / of) * 100)) : 0)

type Zips = ReturnType<typeof useZipUploads>

export function ZipList({ zips }: { zips: Zips }) {
  if (zips.uploads.length === 0) return null
  return (
    <div className="zip-uploads">
      {zips.uploads.map((u) => (
        <ZipCard key={u.id} onRemove={() => void zips.remove(u)} onResume={() => void zips.resume(u)} sendingHere={zips.sending === u.id} upload={u} />
      ))}
    </div>
  )
}

function ZipCard({ upload: u, sendingHere, onResume, onRemove }: { upload: ZipUpload; sendingHere: boolean; onResume: () => void; onRemove: () => void }) {
  const receiving = u.status === 'receiving'
  const percent = receiving ? pct(u.received, u.size) : pct(u.processed, u.total)
  return (
    <div className={`zip-upload zip-upload--${u.status}`}>
      <div className="zip-upload__head">
        <strong>{u.file_name}</strong>
        <small>{mb(u.size)} · {u.created_by}</small>
      </div>
      {(receiving || u.status === 'processing') && (
        <div className="upload-status__bar"><span style={{ width: `${percent}%` }} /></div>
      )}
      <p className="form-hint">
        {receiving && sendingHere && `Enviando: ${percent}% (${mb(u.received)} de ${mb(u.size)}). Pode continuar usando a tela, mas não feche esta aba.`}
        {receiving && !sendingHere && `O envio parou em ${percent}% (${mb(u.received)} de ${mb(u.size)}). Escolha o mesmo arquivo em "Enviar .zip" para continuar de onde parou.`}
        {u.status === 'processing' && u.running && `Gravando as imagens no servidor: ${fmt(u.processed)} de ${fmt(u.total)}. Segue mesmo se você fechar a página.`}
        {u.status === 'processing' && !u.running && `A gravação parou em ${fmt(u.processed)} de ${fmt(u.total)} (o servidor reiniciou?).`}
        {u.status === 'failed' && `Não deu para gravar: ${u.error}`}
        {u.status === 'done' && `Concluído: ${fmt(u.total)} imagem(ns) no zip.`}
      </p>
      {u.status !== 'receiving' && u.processed > 0 && (
        <p className="form-hint">
          {fmt(u.created)} nova(s)
          {u.duplicates > 0 && ` · ${fmt(u.duplicates)} já existiam`}
          {u.error_count > 0 && ` · ${fmt(u.error_count)} com erro`}
          {u.decrypted > 0 && ` · ${fmt(u.decrypted)} criptografada(s), aberta(s)`}
          {u.with_ini > 0 && ` · ${fmt(u.with_ini)} com INI (${fmt(u.prefilled)} com rascunho)`}
          {u.skipped > 0 && ` · ${fmt(u.skipped)} arquivo(s) que não são imagem, ignorado(s)`}
        </p>
      )}
      {u.errors.slice(0, 5).map((message) => <p className="import-report__detail import-report__detail--warning" key={message}>{message}</p>)}
      {u.error_count > 5 && <p className="import-report__detail">… e mais {fmt(u.error_count - 5)}.</p>}
      <div className="zip-upload__actions">
        {u.status === 'processing' && !u.running && <button className="button button--primary button--sm" onClick={onResume} type="button">Continuar a gravação</button>}
        {!sendingHere && !u.running && (
          <button className="button button--secondary button--sm" onClick={onRemove} type="button">
            {u.status === 'done' ? 'Fechar' : 'Cancelar envio'}
          </button>
        )}
      </div>
    </div>
  )
}
