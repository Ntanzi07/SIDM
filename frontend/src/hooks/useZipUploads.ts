import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { cancelZipUpload, finishZipUpload, getZipUpload, listZipUploads, sendZip, type ZipUpload } from '../services/api'
import { notify } from '../services/toast'

// O envio de um .zip em partes, no detalhe do pacote: o estado (os envios
// do pacote, o que esta aba está mandando) e as ações. A tela põe o botão
// (ZipButton) e a lista (ZipList) onde quiser.
export function useZipUploads(packageName: string, iniProfile: string | undefined, onChanged: () => void) {
  const [uploads, setUploads] = useState<ZipUpload[]>([])
  const [sending, setSending] = useState('') // id do envio desta aba
  const changed = useRef(onChanged)
  useEffect(() => { changed.current = onChanged }, [onChanged])

  const put = useCallback((upload: ZipUpload) => {
    setUploads((all) => [upload, ...all.filter((u) => u.id !== upload.id)])
  }, [])

  useEffect(() => {
    let alive = true
    listZipUploads(packageName).then((items) => { if (alive) setUploads(items) }).catch(() => { /* só informativo */ })
    return () => { alive = false }
  }, [packageName])

  // a gravação no servidor: acompanha enquanto roda
  const running = uploads.filter((u) => u.status === 'processing' && u.running).map((u) => u.id).join(',')
  useEffect(() => {
    if (!running) return
    const ids = running.split(',')
    const timer = window.setInterval(() => {
      for (const id of ids) {
        getZipUpload(id).then((upload) => {
          put(upload)
          if (upload.status !== 'processing' || !upload.running) changed.current()
        }).catch(() => { /* tenta de novo no próximo */ })
      }
    }, 2000)
    return () => window.clearInterval(timer)
  }, [running, put])

  async function choose(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = '' // escolher o mesmo arquivo de novo dispara o change
    if (!file) return
    if (!file.name.toLowerCase().endsWith('.zip')) {
      notify('Escolha um arquivo .zip.', 'error')
      return
    }
    try {
      await sendZip(packageName, file, iniProfile, (upload) => { setSending(upload.id); put(upload) })
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível enviar o zip.', 'error')
    } finally {
      setSending('')
    }
  }

  async function resume(upload: ZipUpload) {
    try {
      put(await finishZipUpload(upload.id))
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível continuar.', 'error')
    }
  }

  async function remove(upload: ZipUpload) {
    try {
      await cancelZipUpload(upload.id)
      setUploads((all) => all.filter((u) => u.id !== upload.id))
    } catch (error: unknown) {
      notify(error instanceof Error ? error.message : 'Não foi possível apagar o envio.', 'error')
    }
  }

  return { uploads, sending, choose, resume, remove }
}

