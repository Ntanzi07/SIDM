import type { ImportReport } from '../../services/api'

// Os avisos do ensaio de import, iguais na criação e no detalhe do pacote.
export function ImportWarnings({ report }: { report: ImportReport }) {
  const hasWarnings = report.error_count > 0 || report.not_found_count > 0 || report.invalid_names.length > 0
    || Object.keys(report.unmapped).length > 0 || report.ambiguous.length > 0
  return (
    <>
      {hasWarnings && (
        <p className="import-report__warning">
          Revise os avisos antes de aplicar. O ensaio não alterou nenhuma imagem.
        </p>
      )}
      {report.not_found_count > 0 && (
        <p className="import-report__detail import-report__detail--warning">
          <strong>Sem imagem no pacote ({report.not_found_count.toLocaleString('pt-BR')}):</strong>{' '}
          {report.not_found.slice(0, 3).join(', ')}{report.not_found_count > 3 && ', …'}
          {' '}— a coluna de identificação do perfil é comparada com o nome original do arquivo (ou o sha1).
        </p>
      )}
      {report.ambiguous.length > 0 && (
        <p className="import-report__detail import-report__detail--warning">
          <strong>Nome repetido no pacote:</strong> {report.ambiguous.slice(0, 3).join(', ')}
        </p>
      )}
      {Object.entries(report.unmapped).map(([field, values]) => (
        <p className="import-report__detail" key={field}>
          <strong>{field}:</strong> valores sem tradução ({Object.keys(values).length}): {Object.keys(values).slice(0, 5).join(', ')}
        </p>
      ))}
      {(report.new_equipments.length > 0 || report.new_cameras.length > 0) && (
        <p className="import-report__detail">
          <strong>Cadastros novos:</strong> {[...report.new_equipments, ...report.new_cameras].join(', ')}
        </p>
      )}
      {report.errors.slice(0, 5).map((item) => (
        <p className="import-report__detail import-report__detail--warning" key={`${item.line}-${item.field}-${item.key}`}>
          Linha {item.line}, {item.field}: {item.error}
        </p>
      ))}
      {report.error_count > 5 && (
        <p className="import-report__detail">… e mais {(report.error_count - 5).toLocaleString('pt-BR')} erro(s).</p>
      )}
      {report.invalid_names.slice(0, 5).map((item) => (
        <p className="import-report__detail import-report__detail--warning" key={`${item.line}-${item.field}-${item.value}`}>
          Linha {item.line}, {item.field}: {item.error}
        </p>
      ))}
    </>
  )
}
