import type { UploadProgress } from '../../services/api'

// Progresso do upload em lotes, e os erros (os primeiros, para não lotar a tela).
export function UploadStatus({ progress }: { progress: UploadProgress }) {
  const percent = progress.total ? Math.round((progress.sent / progress.total) * 100) : 0
  const done = progress.sent === progress.total
  return (
    <div className="upload-status">
      <div className="upload-status__bar">
        <span style={{ width: `${percent}%` }} />
      </div>
      <p className="form-hint">
        {done ? 'Envio concluído' : 'Enviando'}: {progress.sent.toLocaleString('pt-BR')} de {progress.total.toLocaleString('pt-BR')}
        {' · '}{progress.created.toLocaleString('pt-BR')} nova(s)
        {progress.duplicates > 0 && ` · ${progress.duplicates.toLocaleString('pt-BR')} já existiam`}
        {progress.errors.length > 0 && ` · ${progress.errors.length.toLocaleString('pt-BR')} com erro`}
      </p>
      {progress.decrypted > 0 && (
        <p className="form-hint">{progress.decrypted.toLocaleString('pt-BR')} criptografada(s), aberta(s) no envio</p>
      )}
      {progress.withIni > 0 && (
        <p className="form-hint">
          {progress.withIni.toLocaleString('pt-BR')} com INI embutido
          {' · '}{progress.prefilled.toLocaleString('pt-BR')} com rascunho preenchido
        </p>
      )}
      {progress.warnings.slice(0, 3).map((message) => (
        <p className="import-report__detail import-report__detail--warning" key={message}>{message}</p>
      ))}
      {progress.warnings.length > 3 && (
        <p className="import-report__detail">… e mais {progress.warnings.length - 3} aviso(s).</p>
      )}
      {progress.errors.slice(0, 5).map((message) => (
        <p className="import-report__detail import-report__detail--warning" key={message}>{message}</p>
      ))}
      {progress.errors.length > 5 && (
        <p className="import-report__detail">… e mais {progress.errors.length - 5}.</p>
      )}
    </div>
  )
}
