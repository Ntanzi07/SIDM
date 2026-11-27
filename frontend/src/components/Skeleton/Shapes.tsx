import type { ReactNode } from 'react'
import { Bone } from './Skeleton'

// As peças que se repetem nos esqueletos das telas. Cada uma usa as classes
// da peça de verdade (o mesmo lugar e o mesmo tamanho); só o conteúdo vira
// bloco. Ficam dentro de um <Skeleton>, que avisa o leitor de tela.

const pick = <T,>(list: readonly T[], i: number) => list[i % list.length]

// O título da tela quando ele ainda não é conhecido (o nome do pacote...).
export function SkTitle({ title = 320, sub = 260, eyebrow = false }: { title?: number | string; sub?: number | string | false; eyebrow?: boolean }) {
  return (
    <div className="sk-title">
      {eyebrow && <Bone h={10} w={140} />}
      <Bone h={40} r={8} w={title} />
      {sub !== false && <Bone h={14} w={sub} />}
    </div>
  )
}

// Botões no canto do título.
export function SkButtons({ widths }: { widths: number[] }) {
  return (
    <div className="sk-buttons">
      {widths.map((w, i) => <Bone h={40} key={i} r={8} w={w} />)}
    </div>
  )
}

// Os números do topo (detail-stats): número grande e o nome embaixo.
export function SkStats({ count, className = 'detail-stats' }: { count: number; className?: string }) {
  return (
    <div className={className}>
      {Array.from({ length: count }, (_, i) => (
        <div className="sk-stat" key={i}>
          <Bone h={30} w={pick([90, 64, 76, 58], i)} />
          <Bone h={11} w={pick([70, 92, 60, 84], i)} />
        </div>
      ))}
    </div>
  )
}

// Um cartão lateral (side-section): título, texto e botões.
export function SkSide({ lines = 2, buttons = [104], children }: { lines?: number; buttons?: number[]; children?: ReactNode }) {
  return (
    <section className="side-section">
      <div className="side-section__head"><Bone h={16} w={150} /></div>
      {Array.from({ length: lines }, (_, i) => <Bone h={12} key={i} w={i === lines - 1 ? '62%' : '100%'} />)}
      {children}
      {buttons.length > 0 && (
        <div className="side-section__actions">
          {buttons.map((w, i) => <Bone h={32} key={i} r={8} w={w} />)}
        </div>
      )}
    </section>
  )
}

const NAMES = ['58%', '44%', '66%', '51%', '38%', '62%']

// A lista de imagens do pacote (image-feed): miniatura, nome e status.
export function SkImageFeed({ count = 8 }: { count?: number }) {
  return (
    <ul className="image-feed">
      {Array.from({ length: count }, (_, i) => (
        <li className="image-feed__item sk-feed__item" key={i}>
          <Bone h={48} r={6} w={72} />
          <Bone h={13} w={pick(NAMES, i)} />
          <span className="sk-feed__status"><Bone h={12} w={74} /></span>
        </li>
      ))}
    </ul>
  )
}

// A coluna de perfis/modelos à esquerda (Exportar, Processar).
export function SkProfileList({ count = 4 }: { count?: number }) {
  return (
    <aside className="export-profiles">
      <Bone h={10} w={64} />
      {Array.from({ length: count }, (_, i) => (
        <div className="sk-profile" key={i}>
          <Bone h={14} w={pick([120, 96, 140, 108], i)} />
          <Bone h={11} w={pick([80, 104, 70, 90], i)} />
        </div>
      ))}
      <Bone h={38} r={8} w="100%" />
    </aside>
  )
}

// Um formulário num cartão: título, texto e campos em grade.
export function SkFormCard({ fields = 6, blocks = 1 }: { fields?: number; blocks?: number }) {
  return (
    <div className="create-card settings-card sk-form">
      <Bone h={24} w={220} />
      <Bone h={13} w="70%" />
      {Array.from({ length: blocks }, (_, b) => (
        <div className="sk-form__grid" key={b}>
          {Array.from({ length: fields }, (_, i) => (
            <div className="sk-field" key={i}>
              <Bone h={11} w={pick([90, 120, 76, 104], i)} />
              <Bone h={40} r={8} w="100%" />
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

// Linhas de rótulo e campo (o formulário da anotação, os dados no visualizador).
export function SkFields({ count = 8, columns = 2 }: { count?: number; columns?: number }) {
  return (
    <div className="sk-fields" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
      {Array.from({ length: count }, (_, i) => (
        <div className="sk-field" key={i}>
          <Bone h={10} w={pick([70, 96, 58, 84], i)} />
          <Bone h={38} r={6} w="100%" />
        </div>
      ))}
    </div>
  )
}
