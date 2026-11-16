import { useCallback, useEffect, useMemo, useState } from 'react'
import { defaultVehicle } from '../data/annotationOptions'
import { createCamera, createEquipment, deleteImage, getCodes, getImage, imageFileUrl, listPackageImages, saveAnnotation, type CodesResponse, type ImageSummary } from '../services/api'
import type { AnnotationForm, ImageDetail, Vehicle } from '../types/annotation'

// Resolution code lookup by WxH string (must match backend codes.go)
const RESOLUTION_BY_SIZE: Record<string, string> = {
  '640x480': '01',
  '752x480': '02',
  '800x600': '03',
  '1024x768': '04',
  '1280x720': '05',
  '1280x960': '06',
  '1712x876': '07',
  '1920x1080': '08',
  '2488x2048': '09',
  '4096x2160': '10',
}

function guessResolutionCode(width: number, height: number): string {
  const key = `${width}x${height}`
  return RESOLUTION_BY_SIZE[key] ?? ''
}

function detectImageResolution(url: string): Promise<string> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => resolve(guessResolutionCode(img.naturalWidth, img.naturalHeight))
    img.onerror = () => resolve('')
    img.src = url
  })
}

function guessDayTime(capturedAt: string): 'D' | 'N' {
  // capturedAt is ISO or datetime-local string — extract hour
  const hour = parseInt(capturedAt.slice(11, 13), 10)
  if (isNaN(hour)) return 'D'
  // "Diurno" = 06:00–17:59, "Noturno" = 18:00–05:59
  return hour >= 6 && hour < 18 ? 'D' : 'N'
}

const initialForm: AnnotationForm = {
  equipment: '',
  camera: '',
  direction: 'T',
  resolution: '',
  capture_angle: 'L',
  day_time: 'D',
  image_range: 'E',
  weather: 'S',
  conditions: ['01'],
  captured_at: new Date().toISOString().slice(0, 16),
  vehicles: [defaultVehicle()],
}

function formFromImage(image: ImageDetail): AnnotationForm {
  const capturedAt = image.captured_at ? image.captured_at.slice(0, 16) : initialForm.captured_at
  return {
    equipment: image.equipment ?? '',
    camera: image.camera ?? '',
    direction: image.direction ?? 'T',
    resolution: image.resolution ?? '',
    capture_angle: image.capture_angle ?? 'L',
    day_time: image.day_time ?? guessDayTime(capturedAt),
    image_range: image.image_range ?? 'E',
    weather: image.weather ?? 'S',
    conditions: image.conditions?.length ? image.conditions : ['01'],
    captured_at: capturedAt,
    vehicles: image.vehicles?.length ? image.vehicles : [defaultVehicle()],
  }
}

export function useAnnotator(packageName: string) {
  const [images, setImages] = useState<ImageSummary[]>([])
  const [codes, setCodes] = useState<CodesResponse>({ equipments: [], cameras: [] })
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [image, setImage] = useState<ImageDetail>()
  const [form, setForm] = useState<AnnotationForm>(initialForm)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const selected = images[selectedIndex]

  function reloadCodes() {
    getCodes()
      .then((response) => setCodes({ ...response }))
      .catch(() => { /* silently ignore */ })
  }

  useEffect(() => {
    Promise.all([listPackageImages(packageName), getCodes()])
      .then(([items, response]) => {
        setImages(items)
        setCodes({ ...response })
        setSelectedIndex(0)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as imagens.'))
      .finally(() => setLoading(false))
  }, [packageName])

  useEffect(() => {
    if (!selected) return
    getImage(selected.sha1)
      .then(async (detail) => {
        const base = formFromImage(detail)
        // Auto-detect resolution from the image file when not set
        if (!detail.resolution) {
          const detectedCode = await detectImageResolution(imageFileUrl(detail.sha1))
          base.resolution = detectedCode
        }
        // Auto-detect day_time from captured_at when not set
        if (!detail.day_time && base.captured_at) {
          base.day_time = guessDayTime(base.captured_at)
        }
        setImage(detail)
        setForm(base)
        setMessage('')
        setError('')
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar a imagem.'))
  }, [selected])

  const updateForm = useCallback(<K extends keyof AnnotationForm>(field: K, value: AnnotationForm[K]) => {
    setForm((current) => ({ ...current, [field]: value }))
  }, [])

  const updateVehicle = useCallback((index: number, patch: Partial<Vehicle>) => {
    setForm((current) => ({
      ...current,
      vehicles: current.vehicles.map((vehicle, vehicleIndex) => vehicleIndex === index ? { ...vehicle, ...patch } : vehicle),
    }))
  }, [])

  const addVehicle = useCallback(() => {
    setForm((current) => ({ ...current, vehicles: [...current.vehicles, defaultVehicle()] }))
  }, [])

  const removeVehicle = useCallback((index: number) => {
    setForm((current) => ({
      ...current,
      vehicles: current.vehicles.length > 1 ? current.vehicles.filter((_, i) => i !== index) : current.vehicles,
    }))
  }, [])

  // Register a new equipment or camera if it doesn't exist yet, then refresh codes
  const ensureRegistry = useCallback(async (type: 'equipment' | 'camera', name: string) => {
    const list = type === 'equipment' ? codes.equipments : codes.cameras
    if (!name || list.includes(name)) return
    try {
      if (type === 'equipment') await createEquipment(name)
      else await createCamera(name)
      reloadCodes()
    } catch {
      // If it already exists (race/duplicate) ignore
    }
  }, [codes.equipments, codes.cameras])

  const deleteCurrentImage = useCallback(async () => {
    if (!image) return false
    setSaving(true)
    setError('')
    try {
      await deleteImage(image.sha1)
      setImages((current) => current.filter((item) => item.sha1 !== image.sha1))
      setImage(undefined)
      setForm(initialForm)
      setSelectedIndex((current) => Math.max(0, current - 1))
      setMessage('Imagem excluída.')
      return true
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível excluir a imagem.')
      return false
    } finally {
      setSaving(false)
    }
  }, [image])

  const save = useCallback(async () => {
    if (!image) return
    const vehicles = form.vehicles.map((vehicle) => ({ ...vehicle, plate: vehicle.plate.replace(/\s/g, '').toUpperCase() }))
    if (!form.equipment || !form.camera || !form.resolution || !form.captured_at || vehicles.some((vehicle) => !vehicle.plate)) {
      setError('Preencha equipamento, câmera, resolução, data/hora e a placa de todos os veículos.')
      return
    }
    setSaving(true)
    setError('')
    try {
      // Auto-register equipment/camera if not yet in the list
      await ensureRegistry('equipment', form.equipment)
      await ensureRegistry('camera', form.camera)
      const saved = await saveAnnotation(image.sha1, { ...form, vehicles })
      setImage(saved)
      setForm(formFromImage(saved))
      setImages((current) => current.map((item) => item.sha1 === saved.sha1 ? { ...item, status: 'verified' } : item))
      setMessage('Anotação salva com sucesso.')
      if (selectedIndex < images.length - 1) setSelectedIndex((current) => current + 1)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar a anotação.')
    } finally {
      setSaving(false)
    }
  }, [form, image, images.length, selectedIndex, ensureRegistry])

  const progress = useMemo(() => ({
    done: images.filter((item) => item.status === 'verified').length,
    total: images.length,
  }), [images])

  const activeImage = image && image.sha1 === selected?.sha1 ? image : undefined
  return { images, codes, selected, selectedIndex, setSelectedIndex, image: activeImage, form, updateForm, updateVehicle, addVehicle, removeVehicle, deleteCurrentImage, save, loading, saving, error, message, progress }
}
