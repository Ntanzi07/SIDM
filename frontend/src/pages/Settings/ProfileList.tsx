import type { ReactNode } from 'react'
import type { AnnotationProfile, ProfileField } from '../../types/annotation'
import './ProfileList.css'

// A lista dos perfis de anotação. Cada perfil mostra o que se anota nele (os
// campos em etiquetas, como aparecem na anotação), e quem estende outro fica
// embaixo dele, ligado por um fio: a herança se vê sem abrir nada.

type Props = {
  profiles: AnnotationProfile[]
  onOpen: (profile: AnnotationProfile) => void
  onDuplicate: (profile: AnnotationProfile) => void
  onRemove: (name: string) => void
}

const MAX_CHIPS = 7

function Chips({ fields }: { fields: ProfileField[] }) {
  const shown = fields.slice(0, MAX_CHIPS)
  const rest = fields.length - shown.length
  return (
    <ul className="pl-chips">
      {shown.map((f) => (
        <li className={f.from ? 'pl-chip pl-chip--inherited' : 'pl-chip'} key={`${f.from ?? ''}${f.key}`} title={f.from ? `de ${f.from}` : undefined}>
          {f.label || f.key}
        </li>
      ))}
      {rest > 0 && <li className="pl-chip pl-chip--more">mais {rest}</li>}
    </ul>
  )
}

function Card({ profile, depth, ...actions }: Props & { profile: AnnotationProfile; depth: number }) {
  const { onOpen, onDuplicate, onRemove } = actions
  const own = (fields: ProfileField[]) => fields.filter((f) => !f.from)
  // um filho mostra só o que acrescenta; o herdado já está no card do pai
  const fields = depth > 0 ? own(profile.fields) : profile.fields
  const objects = profile.objects
    .map((o) => ({ ...o, fields: depth > 0 ? own(o.fields) : o.fields }))
    .filter((o) => depth === 0 || !o.from || o.fields.length > 0)

  return (
    <article aria-labelledby={`pl-${profile.name}`} className={depth > 0 ? 'pl-card pl-card--child' : 'pl-card'}>
      <header className="pl-card__head">
        <div className="pl-card__title">
          <h3 id={`pl-${profile.name}`}>{profile.name}</h3>
          {profile.builtin && <span className="profile-type-badge">padrão</span>}
          {depth > 0 && <span className="pl-extends">estende {profile.extends}</span>}
        </div>
        <div className="pl-card__actions">
          <button className="button button--secondary button--sm" onClick={() => onOpen(profile)} type="button">{profile.builtin ? 'Ver' : 'Editar'}</button>
          <button className="button button--secondary button--sm" onClick={() => onDuplicate(profile)} type="button">Duplicar</button>
          {!profile.builtin && (
            <button aria-label={`Apagar ${profile.name}`} className="button button--danger button--sm" onClick={() => onRemove(profile.name)} type="button">Apagar</button>
          )}
        </div>
      </header>
      {profile.description && <p className="pl-card__desc">{profile.description}</p>}

      <dl className="pl-card__body">
        <div className="pl-line">
          <dt>{depth > 0 ? 'Acrescenta na imagem' : 'Na imagem'}</dt>
          <dd>{fields.length ? <Chips fields={fields} /> : <span className="pl-none">{depth > 0 ? 'nada' : 'nenhum campo'}</span>}</dd>
        </div>
        {objects.map((o) => (
          <div className="pl-line" key={o.key}>
            <dt>
              {o.label || o.key}
              <span className="pl-kind">{o.box ? 'caixa' : 'lista'}</span>
            </dt>
            <dd>{o.fields.length ? <Chips fields={o.fields} /> : <span className="pl-none">sem campos</span>}</dd>
          </div>
        ))}
      </dl>
    </article>
  )
}

export function ProfileList(props: Props) {
  const { profiles } = props
  const names = new Set(profiles.map((p) => p.name))
  // raízes: sem pai (ou com um pai que não está na lista)
  const roots = profiles.filter((p) => !p.extends || !names.has(p.extends))
  const childrenOf = (name: string) => profiles.filter((p) => p.extends === name)

  const branch = (profile: AnnotationProfile, depth: number): ReactNode => {
    const kids = childrenOf(profile.name)
    return (
      <li key={profile.name}>
        <Card {...props} depth={depth} profile={profile} />
        {kids.length > 0 && <ul className="pl-tree pl-tree--kids">{kids.map((k) => branch(k, depth + 1))}</ul>}
      </li>
    )
  }

  if (profiles.length === 0) {
    return (
      <div className="pe-empty">
        <strong>Nenhum perfil ainda</strong>
        <span>Crie um perfil ou duplique o padrão veiculos para começar.</span>
      </div>
    )
  }
  return <ul className="pl-tree">{roots.map((p) => branch(p, 0))}</ul>
}
