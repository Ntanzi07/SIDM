import type { BoxImportReport } from '../../services/api'

const fmt = (n: number) => n.toLocaleString('pt-BR')

function labelList(labels: Record<string, number>) {
  return Object.entries(labels)
    .sort(([, a], [, b]) => b - a)
    .map(([label, n]) => `${label} ${fmt(n)}`)
    .join(' · ')
}

// O relatório do ensaio do import de caixas (COCO 1.0): o que vai ser
// gravado e os avisos. Usado na tela da tarefa e no pop-up do pacote.
export function BoxImportSummary({ report }: { report: BoxImportReport }) {
  return (
    <>
    <div className="import-report__stats">
      <span><strong>{fmt(report.file_images)}</strong> imagens no arquivo</span>
      <span><strong>{fmt(report.matched)}</strong> encontradas no SIDM</span>
      <span><strong>{fmt(report.will_apply)}</strong> serão gravadas</span>
      <span><strong>{fmt(report.not_found_count)}</strong> não encontradas</span>
    </div>
    {Object.keys(report.packages).length > 0 && (
      <p className="import-report__detail"><strong>Por pacote:</strong> {labelList(report.packages)}</p>
    )}
    {Object.keys(report.labels).length > 0 && (
      <p className="import-report__detail"><strong>Caixas por label:</strong> {labelList(report.labels)}</p>
    )}
    {report.new_labels.length > 0 && (
      <p className="import-report__detail import-report__detail--warning">
        Labels que a tarefa ainda não tem (serão acrescentadas): {report.new_labels.join(', ')}
      </p>
    )}
    {report.replaced > 0 && (
      <p className="import-report__detail import-report__detail--warning">
        {fmt(report.replaced)} imagem(ns) já estão nesta tarefa: as caixas delas serão substituídas.
      </p>
    )}
    {report.edited_replaced > 0 && (
      <p className="import-report__detail import-report__detail--warning">
        {fmt(report.edited_replaced)} imagem(ns) têm atributos editados no SIDM: o import do CVAT vai sobrescrever.
      </p>
    )}
    {report.attribute_problem_count > 0 && (
      <div className="import-report__detail import-report__detail--warning">
        <strong>{fmt(report.attribute_problem_count)} atributo(s) fora do perfil</strong> (ficam de fora):
        <ul>
          {report.attribute_problems.slice(0, 10).map((problem) => <li key={problem}>{problem}</li>)}
        </ul>
      </div>
    )}
    {report.missing_required > 0 && (
      <p className="import-report__detail import-report__detail--warning">
        {fmt(report.missing_required)} caixa(s) sem atributo obrigatório: dá para completar na tela de anotação.
      </p>
    )}
    {report.untyped_labels.length > 0 && (
      <p className="import-report__detail">
        Labels sem tipo de objeto com caixa no perfil do pacote (atributos guardados como vieram): {report.untyped_labels.join(', ')}
      </p>
    )}
    {report.empty > 0 && (
      <p className="import-report__detail">
        {fmt(report.empty)} imagem(ns) sem nenhuma caixa no arquivo: {report.negatives ? 'entram como "sem objetos".' : 'ignoradas.'}
      </p>
    )}
    {report.not_found_count > 0 && (
      <p className="import-report__detail import-report__detail--warning">
        <strong>Não encontradas no SIDM:</strong> {report.not_found.slice(0, 5).join(', ')}{report.not_found_count > 5 && ', …'}
        {' '}— rotule a partir do zip "Baixar imagens para rotular" (os nomes começam pelo sha1, e é assim que elas são encontradas).
      </p>
    )}
    {report.ambiguous.length > 0 && (
      <p className="import-report__detail import-report__detail--warning">
        <strong>Nome repetido no SIDM (puladas):</strong> {report.ambiguous.slice(0, 5).join(', ')}
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
    </>
  )
}
