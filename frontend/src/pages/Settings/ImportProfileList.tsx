import type { FieldRule, ImportProfile } from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import './ProfileList.css'

// A lista dos perfis de importação, no modelo da dos perfis de anotação:
// cada card diz de onde lê, como acha a imagem e mostra cada ligação
// "campo ← coluna" em etiquetas.

type Props = {
  profiles: ImportProfile[]
  annotationProfiles: AnnotationProfile[]
  defaultAnnotation: string
  busy: boolean
  onOpen: (profile: ImportProfile) => void
  onDuplicate: (profile: ImportProfile) => void
  onRemove: (name: string) => void
}

const DELIMITERS: Record<string, string> = { ';': 'ponto e vírgula', '\t': 'tabulação', '|': 'barra vertical' }

// de onde vem o valor de um campo: a coluna, o valor fixo ou o template
function source(rule: FieldRule): string {
  if (rule.column) return rule.column
  if (rule.value) return `"${rule.value}"`
  return rule.template ?? '?'
}

const MAX_CHIPS = 8

export function ImportProfileList({ profiles, annotationProfiles, defaultAnnotation, busy, onOpen, onDuplicate, onRemove }: Props) {
  if (profiles.length === 0) {
    return (
      <div className="pe-empty">
        <strong>Nenhum perfil de importação</strong>
        <span>Crie um para preencher os rascunhos a partir de um CSV ou do INI gravado nas imagens.</span>
      </div>
    )
  }
  return (
    <ul className="pl-tree">
      {profiles.map((p) => {
        const target = p.annotation_profile || defaultAnnotation
        const ap = annotationProfiles.find((a) => a.name === target)
        const labelOf = (key: string, object?: string) => {
          const fields = object ? ap?.objects.find((o) => o.key === object)?.fields : ap?.fields
          const f = fields?.find((x) => x.key === key)
          return f?.label || key
        }
        const links = [
          ...Object.entries(p.fields ?? {}).map(([key, rule]) => ({ id: key, name: labelOf(key), from: source(rule) })),
          ...Object.entries(p.objects ?? {}).flatMap(([type, rules]) =>
            Object.entries(rules.fields).map(([key, rule]) => ({ id: `${type}.${key}`, name: `${ap?.objects.find((o) => o.key === type)?.label ?? type}: ${labelOf(key, type)}`, from: source(rule) }))),
        ]
        const shown = links.slice(0, MAX_CHIPS)
        const ini = p.reader.type === 'jpeg-ini'
        return (
          <li key={p.name}>
            <article aria-labelledby={`ip-${p.name}`} className="pl-card">
              <header className="pl-card__head">
                <div className="pl-card__title">
                  <h3 id={`ip-${p.name}`}>{p.name}</h3>
                  {p.builtin && <span className="profile-type-badge">padrão</span>}
                </div>
                <div className="pl-card__actions">
                  <button className="button button--secondary button--sm" disabled={busy} onClick={() => onOpen(p)} type="button">{p.builtin ? 'Ver' : 'Editar'}</button>
                  <button className="button button--secondary button--sm" disabled={busy} onClick={() => onDuplicate(p)} type="button">Duplicar</button>
                  {!p.builtin && <button aria-label={`Apagar ${p.name}`} className="button button--danger button--sm" disabled={busy} onClick={() => onRemove(p.name)} type="button">Apagar</button>}
                </div>
              </header>
              {p.description && <p className="pl-card__desc">{p.description}</p>}
              <dl className="pl-card__body">
                <div className="pl-line">
                  <dt>Lê de</dt>
                  <dd className="pl-text">
                    {ini
                      ? 'INI gravado dentro de cada imagem'
                      : `CSV separado por ${DELIMITERS[p.reader.delimiter ?? ''] ?? 'vírgula'}, ${(p.reader.encoding ?? 'utf-8').toUpperCase()}`}
                  </dd>
                </div>
                {!ini && p.match?.column && (
                  <div className="pl-line">
                    <dt>Acha a imagem</dt>
                    <dd className="pl-text">
                      coluna <code>{p.match.column}</code> = {p.match.by === 'sha1' ? 'SHA1 da imagem' : 'nome original do arquivo'}
                    </dd>
                  </div>
                )}
                <div className="pl-line">
                  <dt>Preenche</dt>
                  <dd className="pl-text">perfil de anotação {target}{!ap && ' (não existe mais)'}</dd>
                </div>
                <div className="pl-line">
                  <dt>{links.length === 1 ? '1 campo' : `${links.length} campos`}</dt>
                  <dd>
                    {links.length ? (
                      <ul className="pl-chips">
                        {shown.map((l) => (
                          <li className="pl-chip" key={l.id}>
                            {l.name} <span className="pl-chip__from">← {l.from}</span>
                          </li>
                        ))}
                        {links.length > shown.length && <li className="pl-chip pl-chip--more">mais {links.length - shown.length}</li>}
                      </ul>
                    ) : <span className="pl-none">nenhum ligado</span>}
                  </dd>
                </div>
              </dl>
            </article>
          </li>
        )
      })}
    </ul>
  )
}
