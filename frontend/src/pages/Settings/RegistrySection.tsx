import { useId, useRef, useState } from 'react'
import {
  createCamera,
  createEquipment,
  deleteCamera,
  deleteEquipment,
  reactivateCamera,
  reactivateEquipment,
  renameCamera,
  renameEquipment,
} from '../../services/api'
import { confirmDialog } from '../../services/confirm'
import { notify } from '../../services/toast'
import { SettingsHeader } from './SettingsUI'
import './Registry.css'

// Equipamentos e câmeras: as opções dos campos de cadastro na anotação.
// Um campo só busca e cadastra: digitar filtra a lista; se o nome não
// existe, a primeira linha oferece cadastrar (Enter cadastra); se ele está
// desativado, oferece reativar (cadastrar de novo daria erro).

type Kind = 'equipment' | 'camera'

const TEXT: Record<Kind, { title: string; one: string; Cap: string; a: string; example: string }> = {
  equipment: { title: 'Equipamentos', one: 'equipamento', Cap: 'Equipamento', a: 'o', example: '8071' },
  camera: { title: 'Câmeras', one: 'câmera', Cap: 'Câmera', a: 'a', example: 'Câmera 1' },
}

const API = {
  equipment: { create: createEquipment, rename: renameEquipment, remove: deleteEquipment, reactivate: reactivateEquipment },
  camera: { create: createCamera, rename: renameCamera, remove: deleteCamera, reactivate: reactivateCamera },
}

// sem acento e sem diferença de maiúscula: "camera" acha "Câmera 1"
const fold = (text: string) => text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim()

type Props = {
  kind: Kind
  items: string[]
  inactive: string[]
  usedBy: string // os perfis de anotação que têm o campo
  onChanged: () => Promise<unknown> | void // recarrega as listas
}

export function RegistrySection({ kind, items, inactive, usedBy, onChanged }: Props) {
  const t = TEXT[kind]
  const api = API[kind]
  const uid = useId()
  const input = useRef<HTMLInputElement>(null)
  const [query, setQuery] = useState('')
  const [busy, setBusy] = useState('') // o nome em que se está mexendo
  const [renaming, setRenaming] = useState<{ name: string; value: string } | null>(null)

  const q = fold(query)
  const shown = q ? items.filter((item) => fold(item).includes(q)) : items
  const shownOff = q ? inactive.filter((item) => fold(item).includes(q)) : inactive
  const typed = query.trim()
  const exact = items.find((item) => fold(item) === q)
  const exactOff = inactive.find((item) => fold(item) === q)
  const canCreate = typed !== '' && !exact && !exactOff

  async function run(name: string, action: () => Promise<string | void>) {
    setBusy(name)
    try {
      const message = await action()
      if (message) notify(message, 'success')
      await onChanged()
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível concluir.', 'error')
    } finally {
      setBusy('')
    }
  }

  const create = () => run(typed, async () => {
    const name = await api.create(typed)
    setQuery('')
    input.current?.focus()
    return `${t.Cap} "${name}" cadastrad${t.a}.`
  })

  const reactivate = (name: string) => run(name, async () => {
    await api.reactivate(name)
    setQuery('')
    return `${t.Cap} "${name}" reativad${t.a}.`
  })

  const remove = async (name: string) => {
    if (!(await confirmDialog({
      title: `Apagar ${t.one}?`,
      message: `"${name}" sai da lista. Se alguma imagem usa, fica desativad${t.a} e dá para reativar depois.`,
      confirmLabel: `Apagar ${t.one}`,
      tone: 'danger',
    }))) return
    await run(name, async () => {
      const result = await api.remove(name)
      return result.deactivated
        ? `${t.Cap} "${name}" desativad${t.a}: ${result.images === 1 ? '1 imagem usa' : `${result.images} imagens usam`}.`
        : `${t.Cap} "${name}" apagad${t.a}.`
    })
  }

  const rename = async () => {
    if (!renaming) return
    const value = renaming.value.trim()
    if (!value || value === renaming.name) { setRenaming(null); return }
    await run(renaming.name, async () => {
      await api.rename(renaming.name, value)
      setRenaming(null)
      return `"${renaming.name}" agora é "${value}".`
    })
  }

  return (
    <div className="set-page">
      <SettingsHeader
        description={`As opções do campo ${t.Cap} na anotação (usado por: ${usedBy}).${kind === 'equipment' ? ' Quem anota também pode cadastrar um novo na hora.' : ''}`}
        title={t.title}
      />

      <form className="rg-search" onSubmit={(e) => { e.preventDefault(); if (canCreate) void create(); else if (exactOff) void reactivate(exactOff) }} role="search">
        <label className="sr-only" htmlFor={`${uid}-q`}>Buscar ou cadastrar {t.one}</label>
        <span aria-hidden="true" className="rg-search__icon" />
        <input
          autoComplete="off"
          id={`${uid}-q`}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Escape') setQuery('') }}
          placeholder={`Buscar ou cadastrar ${t.one} (ex.: ${t.example})`}
          ref={input}
          value={query}
        />
        <span className="rg-search__count" aria-live="polite">
          {q ? `${shown.length} de ${items.length}` : `${items.length} ${items.length === 1 ? t.one : t.title.toLowerCase()}`}
        </span>
      </form>

      <ul className="rg-list" aria-label={t.title}>
        {canCreate && (
          <li className="rg-row rg-row--new">
            <span className="rg-row__name">
              <strong>{typed}</strong>
              <small>não está na lista</small>
            </span>
            <button className="button button--primary button--sm" disabled={busy === typed} type="button" onClick={() => void create()}>
              {busy === typed ? 'Cadastrando…' : `Cadastrar ${t.one}`}
            </button>
          </li>
        )}
        {exactOff && (
          <li className="rg-row rg-row--new">
            <span className="rg-row__name">
              <strong>{exactOff}</strong>
              <small>está desativad{t.a}</small>
            </span>
            <button className="button button--primary button--sm" disabled={busy === exactOff} type="button" onClick={() => void reactivate(exactOff)}>Reativar</button>
          </li>
        )}
        {shown.map((item) => (
          <li className="rg-row" key={item}>
            {renaming?.name === item ? (
              <form className="rg-rename" onSubmit={(e) => { e.preventDefault(); void rename() }}>
                <label className="sr-only" htmlFor={`${uid}-rn`}>Novo nome de {item}</label>
                <input
                  autoFocus
                  id={`${uid}-rn`}
                  onChange={(e) => setRenaming({ name: item, value: e.target.value })}
                  onKeyDown={(e) => { if (e.key === 'Escape') setRenaming(null) }}
                  value={renaming.value}
                />
                <button className="button button--primary button--sm" disabled={busy === item} type="submit">Salvar nome</button>
                <button className="button button--secondary button--sm" onClick={() => setRenaming(null)} type="button">Cancelar</button>
              </form>
            ) : (
              <>
                <span className="rg-row__name"><strong>{item}</strong></span>
                <span className="rg-row__actions">
                  <button aria-label={`Renomear ${item}`} className="button button--secondary button--sm" disabled={busy !== ''} onClick={() => setRenaming({ name: item, value: item })} type="button">Renomear</button>
                  <button aria-label={`Apagar ${item}`} className="button button--danger button--sm" disabled={busy !== ''} onClick={() => void remove(item)} type="button">Apagar</button>
                </span>
              </>
            )}
          </li>
        ))}
        {items.length === 0 && !canCreate && (
          <li className="rg-empty">Nenhum{t.a === 'a' ? 'a' : ''} {t.one} cadastrad{t.a}. Digite o nome acima para cadastrar.</li>
        )}
        {items.length > 0 && shown.length === 0 && !canCreate && !exactOff && (
          <li className="rg-empty">Nada com "{typed}".</li>
        )}
      </ul>

      {shownOff.length > 0 && (
        <section className="rg-off" aria-labelledby={`${uid}-off`}>
          <h3 id={`${uid}-off`}>Desativad{t.a}s <span className="rg-off__count">{inactive.length}</span></h3>
          <p>Foram apagad{t.a}s quando alguma imagem ainda usava. Não aparecem na anotação; reative para usar de novo.</p>
          <ul className="rg-list">
            {shownOff.map((item) => (
              <li className="rg-row rg-row--off" key={item}>
                <span className="rg-row__name"><strong>{item}</strong></span>
                <button className="button button--secondary button--sm" disabled={busy !== ''} onClick={() => void reactivate(item)} type="button">Reativar</button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
