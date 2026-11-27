import { useCallback, useEffect, useState } from 'react'
import { cancelZipUpload, getStorage, purgeTrash, type StorageSummary } from '../../services/api'
import { confirmDialog } from '../../services/confirm'
import { notify } from '../../services/toast'
import { Bone, Skeleton } from '../../components/Skeleton/Skeleton'
import { SettingRow, SettingsGroup, SettingsHeader } from './SettingsUI'

const size = (bytes: number) => {
  const gb = bytes / 1024 ** 3
  if (gb >= 1) return `${gb.toLocaleString('pt-BR', { maximumFractionDigits: gb >= 100 ? 0 : 1 })} GB`
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toLocaleString('pt-BR', { maximumFractionDigits: 0 })} MB`
  return `${Math.ceil(bytes / 1024).toLocaleString('pt-BR')} KB`
}

const PURGE_OPTIONS = [
  { days: 90, label: 'Mais de 90 dias' },
  { days: 30, label: 'Mais de 30 dias' },
  { days: 7, label: 'Mais de 7 dias' },
  { days: 0, label: 'Tudo' },
]

// Sistema (admin): o disco das imagens, a lixeira e os envios de zip
// parados. O disco cheio derruba o resto, então o aviso vem antes.
export function SystemSection() {
  const [st, setSt] = useState<StorageSummary | null>(null)
  const [days, setDays] = useState(30)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    getStorage().then(setSt).catch((error: unknown) => notify(error instanceof Error ? error.message : 'Não foi possível ler o disco.', 'error'))
  }, [])
  useEffect(() => { load() }, [load])

  async function purge() {
    const option = PURGE_OPTIONS.find((o) => o.days === days)
    const ok = await confirmDialog({
      title: 'Esvaziar a lixeira',
      message: `Apagar de vez as imagens da lixeira (${(option?.label ?? '').toLowerCase()})? Uma versão antiga não vai mais conseguir trazer essas imagens de volta.`,
      confirmLabel: 'Apagar de vez',
      tone: 'danger',
    })
    if (!ok) return
    setBusy(true)
    try {
      const res = await purgeTrash(days)
      notify(res.images ? `${res.images.toLocaleString('pt-BR')} imagem(ns) apagada(s) de vez (${size(res.bytes)} liberados).` : 'Nada tão antigo na lixeira.', 'success')
      load()
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível esvaziar a lixeira.', 'error')
    } finally {
      setBusy(false)
    }
  }

  async function removeUpload(id: string, name: string) {
    const ok = await confirmDialog({ title: 'Apagar o envio', message: `Apagar o envio de ${name}? As imagens já gravadas ficam; só o zip parado sai do disco.`, confirmLabel: 'Apagar', tone: 'danger' })
    if (!ok) return
    try {
      await cancelZipUpload(id)
      load()
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível apagar o envio.', 'error')
    }
  }

  // a silhueta: o título é o de verdade; o disco e as linhas viram blocos
  if (!st) return (
    <>
      <SettingsHeader description="O espaço do disco das imagens e o que dá para liberar." title="Armazenamento" />
      <Skeleton label="Lendo o disco">
        <div className="set-group">
          <div className="set-group__panel">
            <div className="set-row set-row--stack">
              <div className="set-row__text"><Bone h={14} w={130} /><Bone h={12} w={220} /></div>
              <Bone h={8} r={999} />
              <div className="sk-buttons"><Bone h={12} w={80} /><Bone h={12} w={140} /></div>
            </div>
            {[150, 120, 170].map((w, i) => (
              <div className="set-row" key={i}>
                <div className="set-row__text"><Bone h={14} w={w} /><Bone h={12} w={w + 90} /></div>
                <Bone h={32} r={8} w={120} />
              </div>
            ))}
          </div>
        </div>
      </Skeleton>
    </>
  )
  const used = st.disk.total - st.disk.free
  const percent = Math.round(st.disk.used_percent)
  return (
    <>
      <SettingsHeader
        actions={<button className="button button--secondary button--sm" onClick={load} type="button">Atualizar</button>}
        description="O espaço do disco das imagens e o que dá para liberar."
        title="Armazenamento"
      />

      <SettingsGroup>
        <SettingRow
          help={st.disk.warn ? 'Quase cheio. Com o disco cheio, o upload e o banco param.' : `${size(st.disk.free)} livres de ${size(st.disk.total)}.`}
          label="Disco das imagens"
          stack
        >
          <div className={st.disk.warn ? 'disk disk--warn' : 'disk'}>
            <div aria-label={`${percent}% usado`} aria-valuemax={100} aria-valuemin={0} aria-valuenow={percent} className="disk__bar" role="meter">
              <span style={{ width: `${Math.min(100, st.disk.used_percent)}%` }} />
            </div>
            <div className="disk__legend">
              <span><strong>{percent}%</strong> usado</span>
              <span>{size(used)} de {size(st.disk.total)}</span>
            </div>
          </div>
        </SettingRow>
        {st.disk.warn && (
          <SettingRow help="Esvazie a lixeira, apague envios parados ou mova as imagens para um disco maior (IMAGES_HOST_DIR no .env)." label="Como liberar espaço" />
        )}
      </SettingsGroup>

      <SettingsGroup description="Envios de zip que não terminaram. O zip fica no disco até terminar ou ser apagado." title="Envios parados">
        {st.uploads.count === 0 ? (
          <SettingRow help="Quando um envio parar no meio, ele aparece aqui." label="Nenhum envio parado" />
        ) : (
          st.uploads.items.map((u) => (
            <SettingRow
              help={`${u.package} · ${u.created_by} · ${u.status === 'receiving' ? `parou em ${Math.round((u.received / u.size) * 100)}% do envio` : u.status === 'failed' ? `falhou: ${u.error}` : u.running ? 'gravando agora' : `gravação parada em ${u.processed} de ${u.total}`}`}
              key={u.id}
              label={u.file_name}
            >
              <span className="set-value set-value--muted">{size(u.size)}</span>
              {!u.running && <button className="button button--secondary button--sm" onClick={() => void removeUpload(u.id, u.file_name)} type="button">Apagar</button>}
            </SettingRow>
          ))
        )}
      </SettingsGroup>

      <SettingsGroup description="Imagens apagadas ficam aqui para uma versão poder trazer de volta." title="Lixeira">
        <SettingRow help={st.trash.count ? `${size(st.trash.bytes)} ocupados.` : 'Vazia.'} label={`${st.trash.count.toLocaleString('pt-BR')} imagem(ns)`} />
      </SettingsGroup>

      <SettingsGroup tone="danger" title="Zona de perigo">
        <SettingRow help="Apaga de vez. Uma versão antiga não vai mais conseguir trazer essas imagens de volta." htmlFor="purge-days" label="Esvaziar a lixeira">
          <select disabled={busy} id="purge-days" onChange={(event) => setDays(Number(event.target.value))} value={days}>
            {PURGE_OPTIONS.map((o) => <option key={o.days} value={o.days}>{o.label}</option>)}
          </select>
          <button className="button button--danger button--sm" disabled={busy || st.trash.count === 0} onClick={() => void purge()} type="button">
            {busy ? 'Apagando…' : 'Esvaziar'}
          </button>
        </SettingRow>
      </SettingsGroup>
    </>
  )
}
