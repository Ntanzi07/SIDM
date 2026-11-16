import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { annotationOptions, defaultAnnotation, defaultVehicle } from '../data/annotationOptions'
import {
  createCamera,
  createEquipment,
  deleteImage,
  getCodes,
  getImage,
  getPackage,
  imageFileUrl,
  nextImage,
  releaseImage,
  saveAnnotation,
  searchCameras,
  searchEquipments,
  type CodesResponse,
  type PackageSummary,
} from '../services/api'
import { getUser } from '../services/user'
import type { AnnotationForm, ImageDetail, Vehicle } from '../types/annotation'
import { guessDayTime, isoToLocalInput, localInputToISO } from '../utils/datetime'

// A anotação segue a FILA do backend (POST /packages/{nome}/next): cada
// imagem fica reservada para quem pegou por 15 minutos, então duas pessoas
// nunca anotam a mesma imagem, e o pacote pode ter qualquer tamanho.
//
// "Próxima" pula a imagem (ela vai para o fim da fila). "Anterior" reabre as
// imagens já vistas nesta sessão, guardadas em `history`.

// Detecta a resolução pelo tamanho real da imagem. O valor é o próprio
// tamanho ("1280x960"), se ele estiver na tabela do GET /codes.
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

const emptyForm: AnnotationForm = {
  equipment: '',
  camera: '',
  direction: defaultAnnotation.direction,
  resolution: '',
  capture_angle: defaultAnnotation.capture_angle,
  day_time: defaultAnnotation.day_time,
  image_range: defaultAnnotation.image_range,
  weather: defaultAnnotation.weather,
  conditions: [...defaultAnnotation.conditions],
  captured_at: '',
  vehicles: [defaultVehicle()],
}

function formFromImage(image: ImageDetail): AnnotationForm {
  const capturedAt = isoToLocalInput(image.captured_at)
  return {
    equipment: image.equipment ?? '',
    camera: image.camera ?? '',
    direction: image.direction ?? emptyForm.direction,
    resolution: image.resolution ?? '',
    capture_angle: image.capture_angle ?? emptyForm.capture_angle,
    day_time: image.day_time ?? (guessDayTime(capturedAt) || emptyForm.day_time),
    image_range: image.image_range ?? emptyForm.image_range,
    weather: image.weather ?? emptyForm.weather,
    conditions: image.conditions?.length ? image.conditions : emptyForm.conditions,
    captured_at: capturedAt,
    vehicles: image.vehicles?.length ? image.vehicles.map(withPlateType) : [defaultVehicle()],
  }
}

// Placa Mercosul: AAA1A11 (letra na 5ª posição). Antiga: AAA1111.
// Só preenche quando o tipo da placa veio vazio (ex.: import sem essa coluna).
function withPlateType(vehicle: Vehicle): Vehicle {
  if (vehicle.plate_type) return vehicle
  const plate = vehicle.plate.replace(/[\s-]/g, '').toUpperCase()
  if (/^[A-Z]{3}\d[A-Z]\d{2}$/.test(plate)) return { ...vehicle, plate_type: 'mercosul' }
  if (/^[A-Z]{3}\d{4}$/.test(plate)) return { ...vehicle, plate_type: 'br' }
  return vehicle
}

// Campos com valor fora das opções (ex.: rascunho de um import sem tradução).
function invalidValues(form: AnnotationForm, vehicles: Vehicle[]): string[] {
  const allowed = (options: readonly (readonly [string, string])[]) => new Set(options.map(([value]) => value))
  const checks: Array<[string, string, readonly (readonly [string, string])[]]> = [
    ['direção', form.direction, annotationOptions.direction],
    ['ângulo', form.capture_angle, annotationOptions.capture_angle],
    ['período', form.day_time, annotationOptions.day_time],
    ['alcance', form.image_range, annotationOptions.image_range],
    ['clima', form.weather, annotationOptions.weather],
    ...form.conditions.map((value): [string, string, typeof annotationOptions.condition] => ['condição', value, annotationOptions.condition]),
  ]
  vehicles.forEach((vehicle, i) => {
    const n = vehicles.length > 1 ? ` (veículo ${i + 1})` : ''
    checks.push(
      [`tipo${n}`, vehicle.type, annotationOptions.type],
      [`tipo da placa${n}`, vehicle.plate_type, annotationOptions.plate_type],
      [`categoria${n}`, vehicle.plate_category, annotationOptions.plate_category],
      [`cor${n}`, vehicle.color, annotationOptions.color],
    )
  })
  return checks
    .filter(([, value, options]) => value !== '' && !allowed(options).has(value))
    .map(([label, value]) => `${label} ("${value}")`)
}

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

export function useAnnotator(packageName: string) {
  const [codes, setCodes] = useState<CodesResponse>({})
  const [summary, setSummary] = useState<PackageSummary>()
  const [image, setImage] = useState<ImageDetail | null>(null)
  const [form, setForm] = useState<AnnotationForm>(emptyForm)
  const [history, setHistory] = useState<string[]>([]) // sha1 das imagens vistas, em ordem
  const [position, setPosition] = useState(-1) // índice de `image` em `history`
  const [finished, setFinished] = useState(false) // a fila acabou
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  // A imagem aberta, para devolver à fila ao sair da página.
  const current = useRef<ImageDetail | null>(null)

  // Os tamanhos conhecidos ("640x480"...), a partir do GET /codes.
  const resolutionBySize = useMemo(() => new Set(Object.values(codes.resolution ?? {})), [codes.resolution])

  const refreshSummary = useCallback(() => {
    getPackage(packageName).then(setSummary).catch(() => { /* o progresso é só informativo */ })
  }, [packageName])

  // Mostra uma imagem: monta o formulário e detecta o que der.
  const show = useCallback(async (detail: ImageDetail | null, bySize: Set<string>) => {
    current.current = detail
    setImage(detail)
    if (!detail) {
      setForm(emptyForm)
      return
    }
    const base = formFromImage(detail)
    if (!detail.resolution) base.resolution = await detectResolution(imageFileUrl(detail.sha1), bySize)
    setForm(base)
  }, [])

  // Pega a próxima da fila (pulando `skip`, se vier) e registra no histórico.
  const pull = useCallback(async (skip: string | undefined, bySize: Set<string>) => {
    const detail = await nextImage(packageName, skip)
    setFinished(!detail)
    await show(detail, bySize)
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
    Promise.all([getCodes(), getPackage(packageName)])
      .then(async ([codesResponse, packageSummary]) => {
        if (!active) return
        setCodes(codesResponse)
        setSummary(packageSummary)
        const bySize = new Set(Object.values(codesResponse.resolution ?? {}))
        await pull(undefined, bySize)
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
  }, [packageName, pull])

  const run = useCallback(async (action: () => Promise<void>, fallback: string) => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      await action()
    } catch (requestError: unknown) {
      setError(messageOf(requestError, fallback))
    } finally {
      setSaving(false)
    }
  }, [])

  // Próxima: pula a atual (ela volta no fim da fila).
  const skip = useCallback(() => run(
    () => pull(image?.status === 'pending' ? image.sha1 : undefined, resolutionBySize),
    'Não foi possível pegar a próxima imagem.',
  ), [image, pull, resolutionBySize, run])

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
    await show(detail, resolutionBySize)
    setPosition(position - 1)
  }, 'Não foi possível abrir a imagem anterior.'), [history, image, position, resolutionBySize, run, show])

  // Volta para a imagem mais recente do histórico (depois de usar "Anterior").
  const forward = useCallback(() => run(async () => {
    if (position >= history.length - 1) {
      await pull(undefined, resolutionBySize)
      return
    }
    const detail = await getImage(history[position + 1])
    await show(detail, resolutionBySize)
    setPosition(position + 1)
  }, 'Não foi possível abrir a imagem.'), [history, position, pull, resolutionBySize, run, show])

  const updateForm = useCallback(<K extends keyof AnnotationForm>(field: K, value: AnnotationForm[K]) => {
    setForm((currentForm) => ({ ...currentForm, [field]: value }))
  }, [])

  const updateVehicle = useCallback((index: number, patch: Partial<Vehicle>) => {
    setForm((currentForm) => ({
      ...currentForm,
      vehicles: currentForm.vehicles.map((vehicle, i) => i === index ? { ...vehicle, ...patch } : vehicle),
    }))
  }, [])

  const addVehicle = useCallback(() => {
    setForm((currentForm) => ({ ...currentForm, vehicles: [...currentForm.vehicles, defaultVehicle()] }))
  }, [])

  const removeVehicle = useCallback((index: number) => {
    setForm((currentForm) => ({
      ...currentForm,
      vehicles: currentForm.vehicles.length > 1 ? currentForm.vehicles.filter((_, i) => i !== index) : currentForm.vehicles,
    }))
  }, [])

  const deleteCurrentImage = useCallback(async () => {
    if (!image) return false
    let ok = false
    await run(async () => {
      await deleteImage(image.sha1)
      current.current = null
      setHistory((list) => list.filter((sha) => sha !== image.sha1))
      await pull(undefined, resolutionBySize)
      setMessage('Imagem excluída.')
      ok = true
    }, 'Não foi possível excluir a imagem.')
    return ok
  }, [image, pull, resolutionBySize, run])

  const save = useCallback(() => run(async () => {
    if (!image) return
    const vehicles = form.vehicles.map((vehicle) => ({ ...vehicle, plate: vehicle.plate.replace(/\s/g, '').toUpperCase() }))
    const capturedAt = localInputToISO(form.captured_at)
    const missing = [
      !form.equipment && 'equipamento',
      !form.camera && 'câmera',
      !form.resolution && 'resolução',
      !capturedAt && 'data/hora',
      vehicles.some((vehicle) => !vehicle.plate) && 'placa',
      vehicles.some((vehicle) => !vehicle.type) && 'tipo do veículo',
      vehicles.some((vehicle) => !vehicle.plate_type) && 'tipo da placa',
      vehicles.some((vehicle) => !vehicle.plate_category) && 'categoria da placa',
      vehicles.some((vehicle) => !vehicle.color) && 'cor',
    ].filter(Boolean)
    if (missing.length > 0) {
      throw new Error(`Preencha: ${missing.join(', ')}.`)
    }
    const invalid = invalidValues(form, vehicles)
    if (invalid.length > 0) {
      throw new Error(`Valor inválido em ${invalid.join(', ')}. Escolha uma opção da lista.`)
    }
    // Equipamento e câmera precisam estar cadastrados (o backend confere).
    // Se não estiverem, a pessoa confirma o cadastro aqui mesmo.
    const equipment = await ensureRegistered('equipment', form.equipment)
    const camera = await ensureRegistered('camera', form.camera)
    setForm((currentForm) => ({ ...currentForm, equipment, camera }))
    await saveAnnotation(image.sha1, { ...form, equipment, camera, vehicles }, capturedAt)
    const wasRevisiting = position < history.length - 1
    if (wasRevisiting) {
      // Corrigiu uma anterior: volta para onde estava.
      const detail = await getImage(history[position + 1])
      await show(detail, resolutionBySize)
      setPosition(position + 1)
      refreshSummary()
    } else {
      await pull(undefined, resolutionBySize)
    }
    setMessage('Anotação salva.')
  }, 'Não foi possível salvar a anotação.'), [form, history, image, position, pull, refreshSummary, resolutionBySize, run, show])

  const progress = {
    done: summary?.verified ?? 0,
    total: summary?.total ?? 0,
    pending: summary?.pending ?? 0,
  }

  return {
    codes,
    image,
    form,
    finished,
    canGoBack: position > 0,
    revisiting: position < history.length - 1,
    updateForm,
    updateVehicle,
    addVehicle,
    removeVehicle,
    skip,
    previous,
    forward,
    deleteCurrentImage,
    save,
    loading,
    saving,
    error,
    message,
    progress,
  }
}
