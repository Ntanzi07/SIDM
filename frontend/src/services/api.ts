import type { AnnotationCodes, AnnotationForm, ImageDetail } from '../types/annotation'

const API_BASE_URL = import.meta.env.VITE_API_URL ?? '/api'

export type PackageSummary = {
  name: string
  created_by: string
  created_at: string
  total: number
  verified: number
  pending: number
  in_progress: number
  coverImage?: string
}

export type ImageSummary = {
  sha1: string
  original_name: string
  queued_at: string
  status: 'pending' | 'verified'
}

export type CodesResponse = AnnotationCodes & {
  equipments: string[]
  cameras: string[]
}

export type ImportProfile = {
  name: string
  description?: string
  reader: {
    type: 'csv' | 'jpeg-ini'
    delimiter?: string
    encoding?: string
  }
  match?: {
    column: string
    by: 'original_name' | 'sha1'
  }
  fields?: Record<string, FieldRule>
  vehicles?: {
    mode: 'single' | 'row'
    fields: Record<string, FieldRule>
  }
}

export type FieldRule = {
  column: string
  date_format?: string
  timezone?: string
}

export type CsvInspection = {
  rows: number
  columns: string[]
  sample: Array<Record<string, string>>
  distinct: Record<string, {
    total: number
    truncated: boolean
    values: Array<{ value: string; count: number }>
  }>
}

export type CreateImportProfileInput = {
  name: string
  description?: string
  reader: {
    type: 'csv'
    delimiter?: string
    encoding?: string
  }
  match: {
    column: string
    by: 'original_name' | 'sha1'
  }
  fields: Record<string, FieldRule>
  vehicles?: {
    mode: 'single' | 'row'
    fields: Record<string, FieldRule>
  }
}

export type ImportReport = {
  dry_run: boolean
  package: string
  profile: string
  overwrite: boolean
  rows: number
  records: number
  unmapped: Record<string, Record<string, number>>
  missing_key: number
  errors: Array<{ key: string; line: number; field: string; error: string }>
  error_count: number
  matched: number
  not_found: string[]
  not_found_count: number
  ambiguous: string[]
  skipped_verified: number
  skipped_claimed: number
  unchanged: number
  applied: number
  conflicts: number
  new_equipments: string[]
  new_cameras: string[]
  invalid_names: Array<{ line: number; field: string; value: string; error: string }>
}

type ImagesResponse = {
  items: ImageSummary[]
}

type NameItem = {
  name: string
}

type PageResponse<T> = {
  total: number
  items: T[]
}

type UploadResult = {
  status: 'created' | 'duplicate' | 'error'
  error?: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: {
      ...(init?.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
      'X-User': 'Guilherme Silva',
      ...init?.headers,
    },
  })

  if (!response.ok) {
    const body = await response.json().catch(() => null) as { error?: string } | null
    throw new Error(body?.error ?? `Não foi possível concluir a operação (${response.status}).`)
  }

  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export async function listPackages(): Promise<PackageSummary[]> {
  const response = await request<PageResponse<PackageSummary>>('/packages')
  return Promise.all(response.items.map(async (packageItem) => {
    const images = await request<ImagesResponse>(
      `/images?package=${encodeURIComponent(packageItem.name)}&limit=500`,
    )
    const lastImage = images.items.at(-1)
    return {
      ...packageItem,
      coverImage: lastImage
        ? `${API_BASE_URL}/images/${lastImage.sha1}/file`
        : undefined,
    }
  }))
}

export async function listPackageImages(packageName: string): Promise<ImageSummary[]> {
  const response = await request<ImagesResponse>(
    `/images?package=${encodeURIComponent(packageName)}&limit=500`,
  )
  return response.items
}

export async function getImage(sha1: string): Promise<ImageDetail> {
  return request<ImageDetail>(`/images/${encodeURIComponent(sha1)}`)
}

export function imageFileUrl(sha1: string): string {
  return `${API_BASE_URL}/images/${encodeURIComponent(sha1)}/file`
}

export async function getCodes(): Promise<CodesResponse> {
  const [codes, equipmentPage, cameraPage] = await Promise.all([
    request<AnnotationCodes>('/codes'),
    request<PageResponse<NameItem>>('/equipments?limit=50'),
    request<PageResponse<NameItem>>('/cameras?limit=50'),
  ])
  return {
    ...codes,
    equipments: equipmentPage.items.map((item) => item.name),
    cameras: cameraPage.items.map((item) => item.name),
  }
}

export async function listImportProfiles(): Promise<ImportProfile[]> {
  const response = await request<PageResponse<ImportProfile>>('/import-profiles?limit=100')
  return response.items.filter((profile) => profile.reader.type === 'csv')
}

export async function inspectCsv(file: File, delimiter: string, encoding: string): Promise<CsvInspection> {
  const formData = new FormData()
  formData.append('file', file, file.name)
  const query = new URLSearchParams({ delimiter, encoding })
  return request<CsvInspection>(`/imports/inspect?${query.toString()}`, {
    method: 'POST',
    body: formData,
  })
}

export async function createImportProfile(profile: CreateImportProfileInput): Promise<ImportProfile> {
  return request<ImportProfile>('/import-profiles', {
    method: 'POST',
    body: JSON.stringify(profile),
  })
}

export async function updateImportProfile(name: string, profile: CreateImportProfileInput): Promise<ImportProfile> {
  return request<ImportProfile>(`/import-profiles/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(profile),
  })
}

export async function deleteImportProfile(name: string): Promise<void> {
  await request(`/import-profiles/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
}

export async function importPackageCsv(
  packageName: string,
  profileName: string,
  file: File,
  dryRun: boolean,
  overwrite: boolean,
): Promise<ImportReport> {
  const formData = new FormData()
  formData.append('file', file, file.name)
  const query = new URLSearchParams({
    profile: profileName,
    dry_run: String(dryRun),
    overwrite: String(overwrite),
  })
  return request<ImportReport>(
    `/packages/${encodeURIComponent(packageName)}/import?${query.toString()}`,
    { method: 'POST', body: formData },
  )
}

export async function createEquipment(name: string): Promise<void> {
  await request('/equipments', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export async function createCamera(name: string): Promise<void> {
  await request('/cameras', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export type RegistryDeleteResult = {
  name: string
  deleted: boolean
  deactivated: boolean
  images: number
}

export async function deleteEquipment(name: string): Promise<RegistryDeleteResult> {
  return request<RegistryDeleteResult>(`/equipments/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

export async function renameEquipment(name: string, newName: string): Promise<void> {
  await request(`/equipments/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    body: JSON.stringify({ name: newName }),
  })
}

export async function deleteCamera(name: string): Promise<RegistryDeleteResult> {
  return request<RegistryDeleteResult>(`/cameras/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

export async function renameCamera(name: string, newName: string): Promise<void> {
  await request(`/cameras/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    body: JSON.stringify({ name: newName }),
  })
}

export async function saveAnnotation(sha1: string, annotation: AnnotationForm): Promise<ImageDetail> {
  return request<ImageDetail>(`/images/${encodeURIComponent(sha1)}`, {
    method: 'PUT',
    body: JSON.stringify({ ...annotation, complete: true }),
  })
}

export async function createPackage(name: string, files: FileList): Promise<string> {
  const created = await request<{ name: string }>('/packages', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })

  const formData = new FormData()
  Array.from(files)
    .filter((file) => file.type.startsWith('image/'))
    .forEach((file) => formData.append('files', file, file.name))

  if (formData.has('files')) {
    await uploadPackageImages(created.name, formData)
  }

  return created.name
}

async function uploadPackageImages(packageName: string, formData: FormData): Promise<void> {
  const results = await request<UploadResult[]>(`/images?package=${encodeURIComponent(packageName)}`, {
    method: 'POST',
    body: formData,
  })
  const failed = results.filter((result) => result.status === 'error')
  if (failed.length > 0) {
    throw new Error(failed.map((result) => result.error ?? 'Falha no upload de uma imagem.').join(' '))
  }
}

export async function addPackageImages(packageName: string, files: FileList): Promise<void> {
  const formData = new FormData()
  Array.from(files)
    .filter((file) => file.type.startsWith('image/'))
    .forEach((file) => formData.append('files', file, file.name))
  if (formData.has('files')) await uploadPackageImages(packageName, formData)
}

export async function deleteImage(sha1: string): Promise<void> {
  await request(`/images/${encodeURIComponent(sha1)}`, { method: 'DELETE' })
}

export async function deletePackage(name: string): Promise<void> {
  await request(`/packages/${encodeURIComponent(name)}?confirm=${encodeURIComponent(name)}`, { method: 'DELETE' })
}
