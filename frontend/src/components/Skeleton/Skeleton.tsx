import type { CSSProperties, ReactNode } from 'react'
import './Skeleton.css'

// O esqueleto de uma tela enquanto carrega: a silhueta do que vem (cartões,
// linhas), no mesmo lugar e na mesma grade, no lugar de um anel girando.
// Assim a tela não pula quando os dados chegam.

// Um bloco: um texto, um número, uma capa. w e h em px ou em % (string).
export function Bone({ w = '100%', h = 12, r, className }: { w?: number | string; h?: number | string; r?: number; className?: string }) {
  const style: CSSProperties = { width: w, height: h, ...(r !== undefined ? { borderRadius: r } : {}) }
  return <span aria-hidden="true" className={className ? `bone ${className}` : 'bone'} style={style} />
}

// A região que carrega: avisa o leitor de tela uma vez (o label) e esconde os
// blocos dele. Aparece com um pequeno atraso, para não piscar quando a
// resposta é rápida.
export function Skeleton({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div aria-busy="true" className={className ? `skeleton ${className}` : 'skeleton'} role="status">
      <span className="skeleton__label">{label}…</span>
      <div aria-hidden="true" className="skeleton__shape">{children}</div>
    </div>
  )
}
