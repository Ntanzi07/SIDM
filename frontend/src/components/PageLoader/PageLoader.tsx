import './PageLoader.css'

// A espera de uma tela: um anel girando e o que está carregando. Aparece
// com um pequeno atraso, para não piscar quando a resposta é rápida.
export function PageLoader({ label = 'Carregando', inline = false }: { label?: string; inline?: boolean }) {
  return (
    <div aria-busy="true" aria-live="polite" className={inline ? 'page-loader page-loader--inline' : 'page-loader'} role="status">
      <span aria-hidden="true" className="page-loader__ring" />
      <span className="page-loader__label">{label}…</span>
    </div>
  )
}
