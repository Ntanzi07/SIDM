import { useCallback, useEffect, useRef, useState } from 'react'
import {
  createCamera,
  createEquipment,
  deleteImage,
  getAnnotationProfile,
  getImage,
  getPackage,
  imageFileUrl,
  nextImage,
  releaseImage,
  reopenImage,
  saveAnnotation,
  searchCameras,
  searchEquipments,
  RequestError,
  type PackageSummary,
  type Problem,
} from '../services/api'
import { getUser } from '../services/user'
import type { AnnotationForm, AnnotationProfile, FormValue, ImageDetail } from '../types/annotation'
import { formFromImage, formToAPI, listObjectTypes, newObject } from '../utils/annotationForm'

// A anotação segue a FILA do backend (POST /packages/{nome}/next): cada
// imagem fica reservada para quem pegou por 15 minutos, então duas pessoas
// nunca anotam a mesma imagem, e o pacote pode ter qualquer tamanho.
//
// "Próxima" pula a imagem (ela vai para o fim da fila). "Anterior" reabre as
// imagens já vistas nesta sessão, guardadas em `history`.

// Detecta a resolução pelo tamanho real da imagem ("1280x960"), se o perfil
// tem esse valor entre as opções do campo "resolution".
function detectResolution(url: string, known: Set<string>): Promise<string> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      const size = `${img.naturalWidth}x${img.naturalHeight}`
      resolve(known.has(size) ? size : '')
    }
    img.onerror = () => resolve('')
    img.src = url
  })
}

const emptyProfile: AnnotationProfile = { name: '', fields: [], objects: [] }
const emptyForm: AnnotationForm = { fields: {}, objects: [] }

// Compara nomes de cadastro do jeito que o backend normaliza (espaço vira
// hífen), sem diferenciar maiúscula: "hikvision 233" acha "hikvision-233".
const normalizeName = (name: string) => name.trim().replace(/\s+/g, '-').toLowerCase()

const registries = {
  equipment: { label: 'equipamento', search: searchEquipments, create: createEquipment },
  camera: { label: 'câmera', search: searchCameras, create: createCamera },
} as const

// Garante que o equipamento/câmera existe. Se já existe (mesmo com outra
// maiúscula), devolve o nome cadastrado. Se não existe, PERGUNTA antes de
// cadastrar: um erro de digitação não pode virar cadastro sem ninguém ver.
async function ensureRegistered(kind: keyof typeof registries, typed: string): Promise<string> {
  const registry = registries[kind]
  const found = await registry.search(typed, 50)
  const existing = found.items.find((name) => normalizeName(name) === normalizeName(typed))
  if (existing) return existing

  if (!window.confirm(`"${typed}" não está cadastrado como ${registry.label}. Cadastrar agora?`)) {
    throw new Error(`Escolha um ${registry.label} da lista ou confirme o cadastro de "${typed}".`)
  }
  try {
    return await registry.create(typed)
  } catch (requestError: unknown) {
    // 409: o nome existe, mas está desativado (os ativos teriam sido achados acima).
    const message = requestError instanceof Error ? requestError.message : ''
    if (message.includes('já está cadastrado')) {
      throw new Error(`O ${registry.label} "${typed}" existe, mas está desativado. Reative em Configurar.`, { cause: requestError })
    }
    throw requestError
  }
}

function messageOf(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback
}

// initialSha1: abre esta imagem primeiro (ex.: "Editar" numa verificada),
// em vez da próxima da fila. Depois de salvar, segue pela fila normalmente.
export function useAnnotator(packageName: string, initialSha1?: string) {
  const [profile, setProfile] = useState<AnnotationProfile>(emptyProfile)
  const [summary, setSummary] = useState<PackageSummary>()
  const [image, setImage] = useState<ImageDetail | null>(null)
  const [form, setForm] = useState<AnnotationForm>(emptyForm)
  const [history, setHistory] = useState<string[]>([]) // sha1 das imagens vistas, em ordem
  const [position, setPosition] = useState(-1) // índice de `image` em `history`
  const [finished, setFinished] = useState(false) // a fila acabou
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [problems, setProblems] = useState<Problem[]>([]) // os erros de validação, campo a campo
  const [message, setMessage] = useState('')

  // A imagem aberta, para devolver à fila ao sair da página.
  const current = useRef<ImageDetail | null>(null)

  const refreshSummary = useCallback(() => {
    getPackage(packageName).then(setSummary).catch(() => { /* o progresso é só informativo */ })
  }, [packageName])

  // Mostra uma imagem: monta o formulário pelo perfil e detecta o que der.
  const show = useCallback(async (detail: ImageDetail | null, p: AnnotationProfile) => {
    current.current = detail
    setImage(detail)
    setProblems([])
    if (!detail) {
      setForm(emptyForm)
      return
    }
    const base = formFromImage(p, detail)
    const resolution = p.fields.find((field) => field.key === 'resolution' && field.type === 'choice')
    if (resolution && !base.fields.resolution) {
      const known = new Set((resolution.options ?? []).map((option) => option.value))
      base.fields.resolution = await detectResolution(imageFileUrl(detail.sha1), known)
    }
    setForm(base)
  }, [])

  // Pega a próxima da fila (pulando `skip`, se vier) e registra no histórico.
  const pull = useCallback(async (skip: string | undefined, p: AnnotationProfile) => {
    const detail = await nextImage(packageName, skip)
    setFinished(!detail)
    await show(detail, p)
    if (detail) {
      setHistory((list) => {
        const next = [...list.filter((sha) => sha !== detail.sha1), detail.sha1]
        setPosition(next.length - 1)
        return next
      })
    }
    refreshSummary()
  }, [packageName, refreshSummary, show])

  useEffect(() => {
    let active = true
    getPackage(packageName)
      .then(async (packageSummary) => {
        if (!active) return
        if (!packageSummary) throw new Error(`O pacote "${packageName}" não existe.`)
        const p = await getAnnotationProfile(packageSummary.profile)
        if (!active) return
        setSummary(packageSummary)
        setProfile(p)
        if (initialSha1) {
          const detail = await reopenImage(initialSha1)
          if (!active) return
          setFinished(false)
          await show(detail, p)
          setHistory([detail.sha1])
          setPosition(0)
        } else {
          await pull(undefined, p)
        }
      })
      .catch((requestError: unknown) => { if (active) setError(messageOf(requestError, 'Não foi possível abrir a fila.')) })
      .finally(() => { if (active) setLoading(false) })

    // Ao sair da página ou fechar a aba, devolve a imagem para a fila.
    const release = () => {
      const open = current.current
      if (open?.status === 'pending') releaseImage(open.sha1)
    }
    window.addEventListener('pagehide', release)
    return () => {
      active = false
      window.removeEventListener('pagehide', release)
      release()
    }
  }, [initialSha1, packageName, pull, show])

  const run = useCallback(async (action: () => Promise<void>, fallback: string) => {
    setSaving(true)
    setError('')
    setProblems([])
    setMessage('')
    try {
      await action()
    } catch (requestError: unknown) {
      if (requestError instanceof RequestError && requestError.problems.length > 0) {
        // O erro aparece embaixo de cada campo; no topo, só o aviso.
        const n = requestError.problems.length
        setError(n === 1 ? 'Revise o campo marcado.' : `Revise os ${n} pontos marcados.`)
        setProblems(requestError.problems)
      } else {
        setError(messageOf(requestError, fallback))
      }
    } finally {
      setSaving(false)
    }
  }, [])

  // Próxima: pula a atual (ela volta no fim da fila).
  const skip = useCallback(() => run(
    () => pull(image?.status === 'pending' ? image.sha1 : undefined, profile),
    'Não foi possível pegar a próxima imagem.',
  ), [image, profile, pull, run])

  // Anterior: reabre a imagem vista antes, nesta sessão.
  const previous = useCallback(() => run(async () => {
    if (position <= 0) return
    const sha1 = history[position - 1]
    if (image?.status === 'pending') releaseImage(image.sha1)
    const detail = await getImage(sha1)
    const reservedByOther = detail.status === 'pending' && detail.claimed_by && detail.claimed_by !== getUser()
      && new Date(detail.claimed_until ?? 0) > new Date()
    if (reservedByOther) setMessage(`Esta imagem está com ${detail.claimed_by} agora: dá para ver, mas não salvar.`)
    setFinished(false)
    await show(detail, profile)
    setPosition(position - 1)
  }, 'Não foi possível abrir a imagem anterior.'), [history, image, position, profile, run, show])

  // Volta para a imagem mais recente do histórico (depois de usar "Anterior").
  const forward = useCallback(() => run(async () => {
    if (position >= history.length - 1) {
      await pull(undefined, profile)
      return
    }
    const detail = await getImage(history[position + 1])
    await show(detail, profile)
    setPosition(position + 1)
  }, 'Não foi possível abrir a imagem.'), [history, position, profile, pull, run, show])

  // Um campo da imagem.
  const updateField = useCallback((key: string, value: FormValue) => {
    setForm((currentForm) => ({ ...currentForm, fields: { ...currentForm.fields, [key]: value } }))
  }, [])

  // Um campo de um objeto.
  const updateObject = useCallback((index: number, key: string, value: FormValue) => {
    setForm((currentForm) => ({
      ...currentForm,
      objects: currentForm.objects.map((object, i) => i === index ? { ...object, values: { ...object.values, [key]: value } } : object),
    }))
  }, [])

  const addObject = useCallback((type: string) => {
    const objectType = listObjectTypes(profile).find((t) => t.key === type)
    if (!objectType) return
    setForm((currentForm) => ({ ...currentForm, objects: [...currentForm.objects, newObject(objectType)] }))
  }, [profile])

  // Pode ficar sem nenhum objeto (ex.: imagem sem veículo).
  const removeObject = useCallback((index: number) => {
    setForm((currentForm) => ({ ...currentForm, objects: currentForm.objects.filter((_, i) => i !== index) }))
  }, [])

  const deleteCurrentImage = useCallback(async () => {
    if (!image) return false
    let ok = false
    await run(async () => {
      await deleteImage(image.sha1)
      current.current = null
      setHistory((list) => list.filter((sha) => sha !== image.sha1))
      await pull(undefined, profile)
      setMessage('Imagem excluída.')
      ok = true
    }, 'Não foi possível excluir a imagem.')
    return ok
  }, [image, profile, pull, run])

  const save = useCallback(() => run(async () => {
    if (!image) return
    // Equipamento e câmera precisam estar cadastrados (o backend confere).
    // Se não estiverem, a pessoa confirma o cadastro aqui mesmo. O resto (o
    // que é obrigatório, as opções, os formatos) quem confere é o perfil, no
    // backend: os erros voltam campo a campo.
    const fields = { ...form.fields }
    for (const field of profile.fields) {
      const typed = typeof fields[field.key] === 'string' ? (fields[field.key] as string).trim() : ''
      if (field.type === 'ref' && field.ref && typed) fields[field.key] = await ensureRegistered(field.ref, typed)
    }
    const next = { ...form, fields }
    setForm(next)
    await saveAnnotation(image.sha1, formToAPI(profile, next))
    const wasRevisiting = position < history.length - 1
    if (wasRevisiting) {
      // Corrigiu uma anterior: volta para onde estava.
      const detail = await getImage(history[position + 1])
      await show(detail, profile)
      setPosition(position + 1)
      refreshSummary()
    } else {
      await pull(undefined, profile)
    }
    setMessage('Anotação salva.')
  }, 'Não foi possível salvar a anotação.'), [form, history, image, position, profile, pull, refreshSummary, run, show])

  const progress = {
    done: summary?.verified ?? 0,
    total: summary?.total ?? 0,
    pending: summary?.pending ?? 0,
  }

  return {
    profile,
    image,
    form,
    finished,
    canGoBack: position > 0,
    revisiting: position < history.length - 1,
    updateField,
    updateObject,
    addObject,
    removeObject,
    skip,
    previous,
    forward,
    deleteCurrentImage,
    save,
    loading,
    saving,
    error,
    problems,
    message,
    progress,
  }
}
