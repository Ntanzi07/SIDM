import type { AnnotationCodes, AnnotationForm, ImageDetail } from '../types/annotation'
import { getUser } from './user'

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

export type CodesResponse = AnnotationCodes

// Regra de um campo no perfil. Espelha importer.FieldRule do backend:
// exatamente um de column, value ou template, mais as transformações.
export type FieldRule = {
  column?: string
  value?: string
  template?: string
  map?: Record<string, string>
  split?: string
  date_format?: string
  timezone?: string
}

export type VehicleRules = {
  mode: 'single' | 'row' | 'columns'
  max?: number
  fields: Record<string, FieldRule>
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
  vehicles?: VehicleRules
  created_by?: string
  updated_at?: string
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
    type: 'csv' | 'jpeg-ini'
    delimiter?: string
    encoding?: string
  }
  // Só no CSV: no INI, os dados estão dentro da própria imagem.
  match?: {
    column: string
    by: 'original_name' | 'sha1'
  }
  fields: Record<string, FieldRule>
  vehicles?: VehicleRules
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

export type PageResponse<T> = {
  total: number
  items: T[]
}

type NameItem = {
  name: string
}

type UploadResult = {
  file: string
  status: 'created' | 'duplicate' | 'error'
  decrypted?: string // formato de criptografia aberto no backend (ex.: rc4-legacy)
  ini?: boolean // o arquivo tinha INI embutido (fica guardado no banco)
  prefilled?: boolean // o perfil de INI (ou o nome v2) preencheu o rascunho
  warnings?: string[]
  error?: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: {
      ...(init?.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
      'X-User': getUser() || 'anonymous',
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

// ---------- pacotes ----------

export async function listPackages(): Promise<PackageSummary[]> {
  const response = await request<PageResponse<PackageSummary>>('/packages?limit=100')
  // Para a capa basta UMA imagem por pacote.
  return Promise.all(response.items.map(async (packageItem) => {
    const images = await request<PageResponse<ImageSummary>>(
      `/images?package=${encodeURIComponent(packageItem.name)}&limit=1`,
    ).catch(() => ({ total: 0, items: [] as ImageSummary[] }))
    const cover = images.items[0]
    return { ...packageItem, coverImage: cover ? imageFileUrl(cover.sha1) : undefined }
  }))
}

// Um pacote pelo nome exato (a busca do backend é "contém").
export async function getPackage(name: string): Promise<PackageSummary | undefined> {
  const response = await request<PageResponse<PackageSummary>>(
    `/packages?q=${encodeURIComponent(name)}&limit=100`,
  )
  return response.items.find((item) => item.name === name)
}

export async function listPackageImages(packageName: string, limit = 100, skip = 0): Promise<PageResponse<ImageSummary>> {
  const query = new URLSearchParams({ package: packageName, limit: String(limit), skip: String(skip) })
  return request<PageResponse<ImageSummary>>(`/images?${query.toString()}`)
}

export async function createPackage(name: string): Promise<string> {
  const created = await request<{ name: string }>('/packages', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
  return created.name
}

export async function deletePackage(name: string): Promise<void> {
  await request(`/packages/${encodeURIComponent(name)}?confirm=${encodeURIComponent(name)}`, { method: 'DELETE' })
}

// ---------- fila de anotação ----------

// Pega a próxima imagem livre do pacote e reserva para o usuário (15 min).
// `skip`: a imagem atual vai para o fim da fila. null = não sobrou pendente.
export async function nextImage(packageName: string, skip?: string): Promise<ImageDetail | null> {
  const image = await request<ImageDetail | undefined>(`/packages/${encodeURIComponent(packageName)}/next`, {
    method: 'POST',
    body: JSON.stringify(skip ? { skip } : {}),
  })
  return image ?? null
}

// Devolve a imagem para a fila. keepalive: funciona mesmo ao fechar a aba.
export function releaseImage(sha1: string): void {
  void fetch(`${API_BASE_URL}/images/${encodeURIComponent(sha1)}/release`, {
    method: 'POST',
    headers: { 'X-User': getUser() || 'anonymous' },
    keepalive: true,
  }).catch(() => { /* se falhar, a reserva vence sozinha */ })
}

// ---------- imagens ----------

export async function getImage(sha1: string): Promise<ImageDetail> {
  return request<ImageDetail>(`/images/${encodeURIComponent(sha1)}`)
}

export function imageFileUrl(sha1: string): string {
  return `${API_BASE_URL}/images/${encodeURIComponent(sha1)}/file`
}

// O form guarda a data no formato do <input type="datetime-local">; aqui ela
// vira ISO com fuso, que é o que o backend aceita.
export async function saveAnnotation(sha1: string, annotation: AnnotationForm, capturedAtISO: string | null): Promise<ImageDetail> {
  return request<ImageDetail>(`/images/${encodeURIComponent(sha1)}`, {
    method: 'PUT',
    body: JSON.stringify({ ...annotation, captured_at: capturedAtISO, complete: true }),
  })
}

export async function deleteImage(sha1: string): Promise<void> {
  await request(`/images/${encodeURIComponent(sha1)}`, { method: 'DELETE' })
}

// ---------- upload em lotes ----------

export type UploadProgress = {
  sent: number
  total: number
  created: number
  duplicates: number
  decrypted: number // imagens novas que chegaram criptografadas e foram abertas
  withIni: number // imagens novas com INI embutido
  prefilled: number // imagens novas com rascunho preenchido
  warnings: string[] // ex.: data do INI em formato inesperado
  errors: string[]
}

// Um pedido com milhares de imagens estoura memória e tempo, e se falhar
// perde tudo. Em lotes, cada lote é independente: um erro não derruba o resto.
const BATCH_FILES = 25
const BATCH_BYTES = 64 * 1024 * 1024

function* batches(files: File[]): Generator<File[]> {
  let batch: File[] = []
  let bytes = 0
  for (const file of files) {
    if (batch.length > 0 && (batch.length >= BATCH_FILES || bytes + file.size > BATCH_BYTES)) {
      yield batch
      batch = []
      bytes = 0
    }
    batch.push(file)
    bytes += file.size
  }
  if (batch.length > 0) yield batch
}

// Arquivos que valem a pena mandar. Não dá para filtrar só por "image/*":
// a imagem CRIPTOGRAFADA pelo equipamento não tem tipo de imagem (o navegador
// não sabe o que é), e quem abre e decide é o backend. Então vai tudo que não
// é claramente outra coisa (CSV, texto, arquivos de sistema...).
const NOT_IMAGE = /\.(csv|txt|ini|json|xml|pdf|log|md|zip|rar|7z|db|exe|lnk|url)$/i
const SYSTEM_FILES = new Set(['thumbs.db', 'desktop.ini', '.ds_store'])

export function imageFiles(files: FileList | File[]): File[] {
  return Array.from(files).filter((file) => {
    const name = file.name.toLowerCase()
    if (name.startsWith('.') || SYSTEM_FILES.has(name)) return false
    if (file.type.startsWith('image/')) return true
    return !NOT_IMAGE.test(name) && !file.type.startsWith('text/')
  })
}

// iniProfile: perfil de INI aplicado em cada imagem que sobe (preenche o
// rascunho com o INI embutido no JPEG). Sem ele, o INI é guardado do mesmo
// jeito, e o perfil pode ser aplicado depois (applyIniProfile).
export async function uploadImages(
  packageName: string,
  files: File[],
  onProgress?: (progress: UploadProgress) => void,
  iniProfile?: string,
): Promise<UploadProgress> {
  const progress: UploadProgress = {
    sent: 0, total: files.length, created: 0, duplicates: 0, decrypted: 0, withIni: 0, prefilled: 0, warnings: [], errors: [],
  }
  const query = new URLSearchParams({ package: packageName })
  if (iniProfile) query.set('profile', iniProfile)
  onProgress?.({ ...progress })
  for (const batch of batches(files)) {
    const formData = new FormData()
    batch.forEach((file) => formData.append('files', file, file.name))
    try {
      const results = await request<UploadResult[]>(`/images?${query.toString()}`, {
        method: 'POST',
        body: formData,
      })
      for (const result of results) {
        if (result.status === 'created') {
          progress.created++
          if (result.decrypted) progress.decrypted++
          if (result.ini) progress.withIni++
          if (result.prefilled) progress.prefilled++
          result.warnings?.forEach((warning) => progress.warnings.push(`${result.file}: ${warning}`))
        } else if (result.status === 'duplicate') {
          progress.duplicates++
        } else {
          progress.errors.push(`${result.file}: ${result.error ?? 'falha no upload'}`)
        }
      }
    } catch (requestError: unknown) {
      const message = requestError instanceof Error ? requestError.message : 'falha no upload'
      batch.forEach((file) => progress.errors.push(`${file.name}: ${message}`))
    }
    progress.sent += batch.length
    onProgress?.({ ...progress, errors: [...progress.errors], warnings: [...progress.warnings] })
  }
  return progress
}

// ---------- códigos e cadastros ----------

export async function getCodes(): Promise<CodesResponse> {
  return request<AnnotationCodes>('/codes')
}

// Autocomplete: o backend busca por "contém" e devolve até `limit` nomes.
async function searchNames(path: string, q: string, limit: number): Promise<PageResponse<string>> {
  const query = new URLSearchParams({ q: q.trim().slice(0, 64), limit: String(limit) })
  const response = await request<PageResponse<NameItem>>(`${path}?${query.toString()}`)
  return { total: response.total, items: response.items.map((item) => item.name) }
}

export function searchEquipments(q: string, limit = 50) {
  return searchNames('/equipments', q, limit)
}

export function searchCameras(q: string, limit = 50) {
  return searchNames('/cameras', q, limit)
}

// Lista completa (de 50 em 50, o máximo por página), para a tela de cadastros.
async function listAllNames(path: string): Promise<string[]> {
  const names: string[] = []
  for (let skip = 0; ; skip += 50) {
    const response = await request<PageResponse<NameItem>>(`${path}?limit=50&skip=${skip}`)
    names.push(...response.items.map((item) => item.name))
    if (response.items.length < 50 || names.length >= response.total) return names
  }
}

export const listEquipments = () => listAllNames('/equipments')
export const listCameras = () => listAllNames('/cameras')

// Devolvem o nome como ficou cadastrado ("hikvision 233" -> "hikvision-233").
export async function createEquipment(name: string): Promise<string> {
  const created = await request<{ name: string }>('/equipments', { method: 'POST', body: JSON.stringify({ name }) })
  return created.name
}

export async function createCamera(name: string): Promise<string> {
  const created = await request<{ name: string }>('/cameras', { method: 'POST', body: JSON.stringify({ name }) })
  return created.name
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
  await request(`/equipments/${encodeURIComponent(name)}`, { method: 'PATCH', body: JSON.stringify({ name: newName }) })
}

export async function deleteCamera(name: string): Promise<RegistryDeleteResult> {
  return request<RegistryDeleteResult>(`/cameras/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

export async function renameCamera(name: string, newName: string): Promise<void> {
  await request(`/cameras/${encodeURIComponent(name)}`, { method: 'PATCH', body: JSON.stringify({ name: newName }) })
}

// ---------- perfis de import ----------

async function listProfilesOfType(type: ImportProfile['reader']['type']): Promise<ImportProfile[]> {
  const response = await request<PageResponse<ImportProfile>>('/import-profiles?limit=100')
  return response.items.filter((profile) => profile.reader.type === type)
}

export const listImportProfiles = () => listProfilesOfType('csv')
export const listIniProfiles = () => listProfilesOfType('jpeg-ini')

export async function listAllImportProfiles(): Promise<ImportProfile[]> {
  const response = await request<PageResponse<ImportProfile>>('/import-profiles?limit=100')
  return response.items
}

// Raio-x do INI embutido em algumas imagens de exemplo (até 20): cada imagem
// vira uma linha e cada "Secao.Chave" vira uma coluna, como num CSV.
export async function inspectIni(files: File[]): Promise<CsvInspection> {
  const formData = new FormData()
  files.slice(0, 20).forEach((file) => formData.append('file', file, file.name))
  return request<CsvInspection>('/imports/inspect?type=jpeg-ini', { method: 'POST', body: formData })
}

export async function inspectCsv(file: File, delimiter: string, encoding: string): Promise<CsvInspection> {
  const formData = new FormData()
  formData.append('file', file, file.name)
  const query = new URLSearchParams({ delimiter, encoding })
  return request<CsvInspection>(`/imports/inspect?${query.toString()}`, { method: 'POST', body: formData })
}

export async function createImportProfile(profile: CreateImportProfileInput): Promise<ImportProfile> {
  return request<ImportProfile>('/import-profiles', { method: 'POST', body: JSON.stringify(profile) })
}

// PUT troca o CONTEÚDO; o nome no corpo tem que ser o mesmo da URL.
export async function updateImportProfile(name: string, profile: CreateImportProfileInput): Promise<ImportProfile> {
  return request<ImportProfile>(`/import-profiles/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(profile),
  })
}

// PATCH troca só o NOME.
export async function renameImportProfile(name: string, newName: string): Promise<ImportProfile> {
  return request<ImportProfile>(`/import-profiles/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    body: JSON.stringify({ name: newName }),
  })
}

export async function deleteImportProfile(name: string): Promise<void> {
  await request(`/import-profiles/${encodeURIComponent(name)}`, { method: 'DELETE' })
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

// Aplica um perfil de INI nas imagens PENDENTES do pacote, usando o INI que
// já foi guardado no upload (sem arquivo). Mesmo ensaio/aplicação do CSV.
export async function applyIniProfile(
  packageName: string,
  profileName: string,
  dryRun: boolean,
  overwrite: boolean,
): Promise<ImportReport> {
  const query = new URLSearchParams({
    profile: profileName,
    dry_run: String(dryRun),
    overwrite: String(overwrite),
  })
  return request<ImportReport>(
    `/packages/${encodeURIComponent(packageName)}/import?${query.toString()}`,
    { method: 'POST' },
  )
}
