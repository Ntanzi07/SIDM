import type { AnnotationCodes, AnnotationProfile, ImageDetail } from '../types/annotation'
import { getUser } from './user'

const API_BASE_URL = import.meta.env.VITE_API_URL ?? '/api'

export type PackageSummary = {
  name: string
  profile: string // perfil de anotação
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

// Como achar os objetos de um tipo no arquivo (espelha importer.ObjectRules).
export type ObjectRules = {
  mode: 'single' | 'row' | 'columns'
  max?: number
  key?: string // o campo que diz que o objeto existe (ex.: a placa)
  fields: Record<string, FieldRule>
}

// O nome antigo.
export type VehicleRules = ObjectRules

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
  // O perfil de anotação que este perfil preenche (vazio = veiculos): as
  // chaves de fields e objects são as dele.
  annotation_profile?: string
  fields?: Record<string, FieldRule>
  objects?: Record<string, ObjectRules> // tipo de objeto -> regras
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
  annotation_profile?: string
  fields: Record<string, FieldRule>
  objects?: Record<string, ObjectRules>
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
  // O que o perfil de anotação do pacote não tem ou não aceita (fica de fora).
  outside_profile?: string[]
  outside_profile_count?: number
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

// Um problema de validação do perfil de anotação: path diz onde (ex.:
// "fields.day_time", "objects[0].plate").
export type Problem = { path: string; msg: string }

// O erro de um pedido. Na anotação, problems tem TODOS os erros (a mensagem
// só traz o primeiro).
export class RequestError extends Error {
  problems: Problem[]
  status: number // o código HTTP (ex.: 409 = mudou desde a leitura)
  constructor(message: string, problems: Problem[], status = 0) {
    super(message)
    this.problems = problems
    this.status = status
  }
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
    const body = await response.json().catch(() => null) as { error?: string; problems?: Problem[] } | null
    throw new RequestError(body?.error ?? `Não foi possível concluir a operação (${response.status}).`, body?.problems ?? [], response.status)
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

export async function listPackageImages(
  packageName: string,
  limit = 100,
  skip = 0,
  status?: ImageSummary['status'],
): Promise<PageResponse<ImageSummary>> {
  const query = new URLSearchParams({ package: packageName, limit: String(limit), skip: String(skip) })
  if (status) query.set('status', status)
  return request<PageResponse<ImageSummary>>(`/images?${query.toString()}`)
}

// profile vazio = o perfil de anotação padrão (veiculos).
export async function createPackage(name: string, profile?: string): Promise<string> {
  const created = await request<{ name: string }>('/packages', {
    method: 'POST',
    body: JSON.stringify(profile ? { name, profile } : { name }),
  })
  return created.name
}

// Só enquanto o pacote não tem imagem verificada (o backend recusa).
export async function setPackageProfile(name: string, profile: string): Promise<void> {
  await request(`/packages/${encodeURIComponent(name)}`, { method: 'PATCH', body: JSON.stringify({ profile }) })
}

// ---------- perfis de anotação ----------

export async function listAnnotationProfiles(): Promise<AnnotationProfile[]> {
  return (await request<PageResponse<AnnotationProfile>>('/annotation-profiles')).items
}

export async function getAnnotationProfile(name: string): Promise<AnnotationProfile> {
  return request<AnnotationProfile>(`/annotation-profiles/${encodeURIComponent(name)}`)
}

// Os erros de validação voltam todos juntos (RequestError.problems).
export async function createAnnotationProfile(profile: AnnotationProfile): Promise<AnnotationProfile> {
  return request<AnnotationProfile>('/annotation-profiles', { method: 'POST', body: JSON.stringify(profile) })
}

export async function updateAnnotationProfile(profile: AnnotationProfile): Promise<AnnotationProfile> {
  return request<AnnotationProfile>(`/annotation-profiles/${encodeURIComponent(profile.name)}`, { method: 'PUT', body: JSON.stringify(profile) })
}

export async function deleteAnnotationProfile(name: string): Promise<void> {
  await request(`/annotation-profiles/${encodeURIComponent(name)}`, { method: 'DELETE' })
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

// Abre uma imagem específica para edição e reserva para o usuário. Uma
// VERIFICADA volta para pendente (o nome v2 sai do banco até ser
// verificada de novo); numa pendente, só reserva.
export async function reopenImage(sha1: string): Promise<ImageDetail> {
  return request<ImageDetail>(`/images/${encodeURIComponent(sha1)}/reopen`, { method: 'POST' })
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

// Verifica a imagem com a anotação (já no formato da API: ver
// utils/annotationForm.ts, formToAPI).
export async function saveAnnotation(sha1: string, annotation: { fields: Record<string, unknown>; objects: { type: string; fields: Record<string, unknown> }[] }): Promise<ImageDetail> {
  return request<ImageDetail>(`/images/${encodeURIComponent(sha1)}`, {
    method: 'PUT',
    body: JSON.stringify({ ...annotation, complete: true }),
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

// ---------- caixas (bounding boxes) do CVAT ----------

export type Box = {
  label: string
  x: number // pixels, canto superior esquerdo
  y: number
  w: number
  h: number
  occluded?: boolean
  rotation?: number // graus, sentido horário (CVAT)
  attributes?: Record<string, unknown>
}

export type BoxSet = {
  task: string
  sha1: string
  package: string
  width: number // tamanho da imagem no CVAT: as caixas são em pixels dela
  height: number
  boxes: Box[]
  source: { tool: string; format: string; file: string; file_name: string }
  imported_by: string
  imported_at: string
  updated_by?: string // quem editou os atributos no SIDM (o reimport zera)
  updated_at?: string
}

// A versão das caixas para a edição: a última edição ou, sem edição, o import.
export const boxSetVersion = (set: BoxSet) => set.updated_at ?? set.imported_at

export type BoxImportReport = {
  dry_run: boolean
  task: string
  file: string
  negatives: boolean
  file_images: number
  file_boxes: number
  labels: Record<string, number>
  new_labels: string[]
  matched: number
  packages: Record<string, number>
  not_found: string[]
  not_found_count: number
  ambiguous: string[]
  empty: number
  size_mismatch: Array<{ file_name: string; cvat: string; sidm: string }>
  will_apply: number
  replaced: number
  applied: number
  applied_boxes: number
  warnings: string[]
  // atributos das caixas, conferidos pelo tipo de objeto com caixa do perfil
  attribute_problems: string[]
  attribute_problem_count: number
  missing_required: number
  untyped_labels: string[]
  edited_replaced: number
}

// Por tarefa, quantas imagens do pacote estão anotadas nela.
export type PackageBoxes = {
  package: string
  tasks: Array<{ task: string; images: number; boxes: number }>
}

// O zip com as imagens do pacote, para criar a task no CVAT (os nomes
// começam pelo sha1, e é assim que o export volta casando).
export function cvatZipUrl(packageName: string): string {
  return `${API_BASE_URL}/packages/${encodeURIComponent(packageName)}/cvat.zip`
}

export async function getPackageBoxes(packageName: string): Promise<PackageBoxes> {
  return request<PackageBoxes>(`/packages/${encodeURIComponent(packageName)}/boxes`)
}

// As caixas da imagem, uma entrada por tarefa (vazio: nenhuma).
export async function getImageBoxes(sha1: string): Promise<BoxSet[]> {
  const response = await request<{ items: BoxSet[] }>(`/images/${encodeURIComponent(sha1)}/boxes`)
  return response.items
}

// Edita os atributos das caixas de uma imagem numa tarefa (uma entrada por
// caixa, na ordem do GET). 400 com problems ("boxes[i].attributes.<campo>");
// 409 se as caixas mudaram desde a leitura.
export async function updateBoxAttributes(set: BoxSet, attributes: Array<Record<string, unknown>>): Promise<BoxSet> {
  return request<BoxSet>(`/images/${encodeURIComponent(set.sha1)}/boxes/${encodeURIComponent(set.task)}`, {
    method: 'PUT',
    body: JSON.stringify({ version: boxSetVersion(set), boxes: attributes.map((a) => ({ attributes: a })) }),
  })
}

// ---------- tarefas de anotação de caixas ----------

export type TaskPackage = { name: string; images: number; verified: number }

export type TaskStats = {
  images: number // anotadas na tarefa
  verified: number // dessas, as verificadas (entram no export)
  negatives: number // anotadas sem nenhuma caixa
  boxes: number
  labels: Record<string, number>
  packages: TaskPackage[]
}

export type AnnotationTask = {
  name: string
  description: string
  labels: string[]
  created_by?: string
  created_at?: string
  updated_at?: string
  stats?: TaskStats
}

export type TaskBody = { name: string; description: string; labels: string[] }

export async function listTasks(): Promise<AnnotationTask[]> {
  const response = await request<PageResponse<AnnotationTask>>('/tasks')
  return response.items
}

export async function getTask(name: string): Promise<AnnotationTask> {
  return request<AnnotationTask>(`/tasks/${encodeURIComponent(name)}`)
}

export async function createTask(body: TaskBody): Promise<AnnotationTask> {
  return request<AnnotationTask>('/tasks', { method: 'POST', body: JSON.stringify(body) })
}

// Mudar o nome leva junto as caixas e os perfis de export.
export async function updateTask(oldName: string, body: TaskBody): Promise<AnnotationTask> {
  return request<AnnotationTask>(`/tasks/${encodeURIComponent(oldName)}`, { method: 'PUT', body: JSON.stringify(body) })
}

export async function deleteTask(name: string): Promise<{ deleted: string; images: number }> {
  return request(`/tasks/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

// Export COCO 1.0 do CVAT (o zip, ou só o .json) para a tarefa. As imagens
// são procuradas em todos os pacotes. dryRun: só o relatório. negatives:
// imagens sem caixa entram como anotadas e vazias.
export async function importTaskBoxes(task: string, file: File, dryRun: boolean, negatives: boolean): Promise<BoxImportReport> {
  const formData = new FormData()
  formData.append('file', file, file.name)
  return request<BoxImportReport>(
    `/tasks/${encodeURIComponent(task)}/import?dry_run=${String(dryRun)}&negatives=${String(negatives)}`,
    { method: 'POST', body: formData },
  )
}

// ---------- export (datasets RF-DETR / YOLO) ----------

export type ExportSplit = { train: number; valid: number; test: number }

// Filtros nos campos do perfil de anotação do perfil de export (os valores
// como texto: número "2", sim/não "true").
export type ExportFilters = {
  fields?: Record<string, string> // campo da imagem -> valor
  objects?: Record<string, Record<string, string>> // tipo de objeto -> campo -> valor (o MESMO objeto)
}

// Um nível da árvore de pastas (layout "folders", só VOC).
export type FolderLevel = {
  field: string
  names?: Record<string, string> // valor -> nome da pasta (os que faltam usam o padrão)
  unknown?: string // pasta das imagens sem o metadado (padrão: Desconhecido)
}

export type ExportProfile = {
  name: string
  description?: string
  format: 'coco' | 'yolo' | 'voc'
  layout?: 'split' | 'folders' // split = train/valid/test (padrão)
  folders?: FolderLevel[]
  file_name?: string // modelo do nome dos arquivos; vazio = o nome do SIDM (v2)
  // tarefas de anotação cujas caixas entram, juntas (os padrões não têm: escolhe na hora)
  tasks?: string[]
  // só as imagens anotadas em TODAS as tarefas (sem isso, o objeto não marcado vira fundo)
  all_tasks?: boolean
  // perfil de anotação dos campos dos filtros, das pastas e do nome (vazio = veiculos)
  annotation_profile?: string
  split: ExportSplit
  packages?: string[]
  filters: ExportFilters
  labels: { rename?: Record<string, string>; include?: string[] }
  builtin?: boolean
  created_by?: string
  updated_at?: string
}

export type ExportPreview = {
  profile: string
  task: string // as tarefas juntas, para mostrar ("parabrisa + placa")
  tasks: string[]
  all_tasks: boolean
  format: string
  layout: string
  folders: Record<string, number> // imagens por pasta (layout por pastas)
  examples: string[] // os primeiros nomes de arquivo, como vão sair
  split: ExportSplit
  classes: string[]
  total: number
  negatives: number // das imagens do dataset, as sem nenhuma caixa
  images: Record<string, number>
  boxes: Record<string, number>
  // not_annotated: em nenhuma das tarefas; missing_tasks: com all_tasks, em só parte delas
  skipped: { not_annotated: number; missing_tasks: number; without_labels: number }
  packages: string[]
}

// Um metadado da imagem (de um perfil de anotação) que pode virar pasta ou
// parte do nome, com o nome padrão da pasta de cada valor (values null:
// campo aberto, a pasta é o próprio valor).
export type FolderField = {
  field: string
  label: string
  group: string // file, image ou o tipo do objeto
  values: Record<string, string> | null
  labels?: Record<string, string> // valor -> nome na tela
  folder: boolean
  name: boolean
  alias?: boolean // nome antigo/atalho: aceito, mas não oferecido
}

export type ExportFieldsInfo = {
  annotation_profile: string
  fields: FolderField[]
  unknown: string
  name_fields: string[] // campos do modelo de nome dos arquivos
  legacy_template: string // o padrão antigo (só no perfil veiculos)
}

export async function getExportFolderFields(annotationProfile?: string): Promise<ExportFieldsInfo> {
  const query = annotationProfile ? `?annotation_profile=${encodeURIComponent(annotationProfile)}` : ''
  return request(`/export-folder-fields${query}`)
}

export async function listExportProfiles(): Promise<ExportProfile[]> {
  const response = await request<PageResponse<ExportProfile>>('/export-profiles')
  return response.items
}

export async function createExportProfile(profile: ExportProfile): Promise<ExportProfile> {
  return request<ExportProfile>('/export-profiles', { method: 'POST', body: JSON.stringify(profile) })
}

export async function updateExportProfile(profile: ExportProfile): Promise<ExportProfile> {
  return request<ExportProfile>(`/export-profiles/${encodeURIComponent(profile.name)}`, {
    method: 'PUT',
    body: JSON.stringify(profile),
  })
}

export async function deleteExportProfile(name: string): Promise<void> {
  await request(`/export-profiles/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

// Sem os campos que o servidor preenche (o perfil vai "inline", como está na tela).
function exportBody(profile: ExportProfile) {
  const { builtin: _builtin, created_by: _createdBy, updated_at: _updatedAt, ...rest } = profile
  void _builtin
  void _createdBy
  void _updatedAt
  return rest
}

export async function previewExport(profile: ExportProfile): Promise<ExportPreview> {
  return request<ExportPreview>('/exports/preview', {
    method: 'POST',
    body: JSON.stringify({ profile: exportBody(profile) }),
  })
}

// O zip pode ter gigas: em vez de baixar pelo fetch (que seguraria tudo na
// memória), manda um formulário, e o navegador salva direto no disco.
export function downloadExport(profile: ExportProfile): void {
  // Alvo: um iframe escondido. Assim a página não sai do lugar (nem se a
  // resposta for um erro); a tela libera o download só depois da prévia.
  let frame = document.getElementById('sidm-download-frame') as HTMLIFrameElement | null
  if (!frame) {
    frame = document.createElement('iframe')
    frame.id = 'sidm-download-frame'
    frame.name = 'sidm-download-frame'
    frame.style.display = 'none'
    document.body.appendChild(frame)
  }
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = `${API_BASE_URL}/exports/download`
  form.target = frame.name
  form.style.display = 'none'
  const field = document.createElement('input')
  field.type = 'hidden'
  field.name = 'request'
  field.value = JSON.stringify({ profile: exportBody(profile) })
  form.appendChild(field)
  document.body.appendChild(form)
  form.submit()
  form.remove()
}

// O perfil de anotação de cada pacote (nome -> perfil), sem buscar capa.
export async function listPackageProfiles(): Promise<Record<string, string>> {
  const response = await request<PageResponse<PackageSummary>>('/packages?limit=100')
  return Object.fromEntries(response.items.map((item) => [item.name, item.profile]))
}

// Só os nomes dos pacotes (sem buscar capa).
export async function listPackageNames(): Promise<string[]> {
  const response = await request<PageResponse<PackageSummary>>('/packages?limit=100')
  return response.items.map((item) => item.name)
}
