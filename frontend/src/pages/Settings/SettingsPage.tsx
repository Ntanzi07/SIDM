import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  createCamera,
  createEquipment,
  createImportProfile,
  deleteCamera,
  deleteEquipment,
  deleteImportProfile,
  imageFiles,
  inspectCsv,
  inspectIni,
  listAllImportProfiles,
  listAnnotationProfiles,
  listCameras,
  listEquipments,
  renameCamera,
  renameEquipment,
  renameImportProfile,
  updateImportProfile,
  type CsvInspection,
  type FieldRule,
  type ImportProfile,
  type ObjectRules,
} from '../../services/api'
import type { AnnotationProfile, ProfileField } from '../../types/annotation'
import { AnnotationProfilesSection } from './AnnotationProfilesSection'
import './Settings.css'

const DEFAULT_ANNOTATION_PROFILE = 'veiculos'

// O que a tela guarda de um campo de data e hora: o formato, o fuso e,
// opcionalmente, a hora numa coluna separada (comum no INI: Data= e Hora=).
type DateSetting = { format: string; custom: boolean; timezone: string; timeColumn: string }
type ObjectSetting = { mode: ObjectRules['mode']; max?: number; key?: string }

// Formatos de data mais comuns. O backend usa o layout do Go: a data de
// referência 2006-01-02 15:04:05 escrita do jeito que aparece no CSV.
const DATE_PRESETS = [
  ['02/01/2006 15:04:05', 'DD/MM/AAAA HH:MM:SS'],
  ['02/01/2006 15:04', 'DD/MM/AAAA HH:MM'],
  ['02/01/2006', 'DD/MM/AAAA'],
  ['2006-01-02 15:04:05', 'AAAA-MM-DD HH:MM:SS'],
  ['2006-01-02 15:04:05.000', 'AAAA-MM-DD HH:MM:SS.mmm'],
  ['2006-01-02T15:04:05Z07:00', 'ISO 8601 (com fuso)'],
] as const

// Monta as regras finais do perfil: o que a tela edita (a coluna de cada
// campo) por cima do perfil original, SEM perder o que a tela não mostra
// (map, split, valor fixo, template). Antes, salvar pela tela apagava isso.
function mergeRules(
  original: Record<string, FieldRule> | undefined,
  columns: Record<string, string>,
  editable: readonly string[],
  dates: Record<string, DateSetting> = {},
  datePrefix = '',
): Record<string, FieldRule> {
  const out: Record<string, FieldRule> = {}
  for (const [field, rule] of Object.entries(original ?? {})) {
    // Campos que a tela não edita, ou com valor fixo/template: ficam como estão.
    if (!editable.includes(field) || !rule.column) out[field] = rule
  }
  for (const field of editable) {
    const column = columns[field]
    if (!column) continue
    const previous: FieldRule = { ...original?.[field] }
    delete previous.value
    delete previous.template
    delete previous.column
    const date = dates[datePrefix + field]
    if (date) {
      // Uma coluna só (column) ou data + hora juntas (template "{Data} {Hora}").
      out[field] = {
        ...previous,
        ...(date.timeColumn ? { template: `{${column}} {${date.timeColumn}}` } : { column }),
        date_format: date.format.trim(),
        timezone: date.timezone.trim() || undefined,
      }
    } else {
      out[field] = { ...previous, column }
    }
  }
  return out
}

// Lê as colunas de um conjunto de regras (o caminho de volta do mergeRules).
function readRules(
  rules: Record<string, FieldRule> | undefined,
  datePrefix: string,
  cols: Set<string>,
  dates: Record<string, DateSetting>,
): Record<string, string> {
  const columns: Record<string, string> = {}
  for (const [field, rule] of Object.entries(rules ?? {})) {
    if (rule.column) {
      columns[field] = rule.column
      cols.add(rule.column)
    }
    if (rule.date_format) {
      const date = newDateSetting()
      date.format = rule.date_format
      date.custom = !DATE_PRESETS.some(([layout]) => layout === rule.date_format)
      if (rule.timezone) date.timezone = rule.timezone
      // Data e hora separadas ficam num template "{Data} {Hora}".
      const parts = rule.template?.match(/^\{([^{}]+)\} \{([^{}]+)\}$/)
      if (parts) {
        columns[field] = parts[1]
        date.timeColumn = parts[2]
        cols.add(parts[1])
        cols.add(parts[2])
      }
      dates[datePrefix + field] = date
    }
  }
  return columns
}

function newDateSetting(): DateSetting {
  return { format: DATE_PRESETS[0][0], custom: false, timezone: 'America/Sao_Paulo', timeColumn: '' }
}

const fieldLabel = (field: { key: string; label?: string }) => field.label || field.key

const TYPE_HINTS: Partial<Record<ProfileField['type'], string>> = {
  choice: 'lista: aceita o valor, o código ou o nome da opção',
  multi: 'vários valores da lista',
  number: 'número',
  bool: 'sim/não (true/false, sim/não, 1/0)',
  datetime: 'data e hora',
  ref: 'cadastro: criado se não existir',
}

// Descreve o que um campo tem além da coluna, para a tela avisar que é mantido.
function advancedLabel(rule: FieldRule | undefined): string {
  if (!rule) return ''
  const parts: string[] = []
  if (rule.value) parts.push(`valor fixo "${rule.value}"`)
  if (rule.template) parts.push(`template ${rule.template}`)
  if (rule.map) parts.push(`${Object.keys(rule.map).length} tradução(ões)`)
  if (rule.split) parts.push(`separa por "${rule.split}"`)
  return parts.join(', ')
}

type EditingRegistry = { type: 'equipment' | 'camera'; name: string; newName: string } | null

export function SettingsPage() {
  const navigate = useNavigate()
  const [equipments, setEquipments] = useState<string[]>([])
  const [cameras, setCameras] = useState<string[]>([])
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [editingProfile, setEditingProfile] = useState<string | null>(null)
  const [editingOriginal, setEditingOriginal] = useState<ImportProfile | null>(null)
  // csv: os dados vêm de um CSV. jpeg-ini: do INI gravado dentro de cada JPEG.
  const [profileType, setProfileType] = useState<'csv' | 'jpeg-ini'>('csv')
  const [profileImages, setProfileImages] = useState<File[]>([])
  const [annotationProfiles, setAnnotationProfiles] = useState<AnnotationProfile[]>([])
  // O perfil de anotação que o perfil de import preenche (os campos de destino).
  const [annotationProfileName, setAnnotationProfileName] = useState(DEFAULT_ANNOTATION_PROFILE)
  // Formato de cada campo de data mapeado ("captured_at" ou "<tipo>.<campo>").
  const [dateSettings, setDateSettings] = useState<Record<string, DateSetting>>({})
  const [availableColumns, setAvailableColumns] = useState<string[]>([])
  const [equipmentName, setEquipmentName] = useState('')
  const [cameraName, setCameraName] = useState('')
  const [profileName, setProfileName] = useState('')
  const [profileDescription, setProfileDescription] = useState('')
  const [profileFile, setProfileFile] = useState<File | null>(null)
  const [delimiter, setDelimiter] = useState('semicolon')
  const [encoding, setEncoding] = useState('utf-8')
  const [inspection, setInspection] = useState<CsvInspection | null>(null)
  const [matchColumn, setMatchColumn] = useState('')
  const [matchBy, setMatchBy] = useState<'original_name' | 'sha1'>('original_name')
  const [fieldColumns, setFieldColumns] = useState<Record<string, string>>({})
  // tipo de objeto -> campo -> coluna
  const [objectColumns, setObjectColumns] = useState<Record<string, Record<string, string>>>({})
  const [objectSettings, setObjectSettings] = useState<Record<string, ObjectSetting>>({})
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [saving, setSaving] = useState<'equipment' | 'camera' | 'inspect' | 'profile' | 'registry' | ''>('')
  const [activeTab, setActiveTab] = useState<'equipments' | 'cameras' | 'annotation_profiles' | 'profiles_list' | 'profile_editor'>('equipments')

  // Inline edit/rename state for equipment and camera
  const [editingRegistry, setEditingRegistry] = useState<EditingRegistry>(null)

  function loadCodes() {
    Promise.all([listEquipments(), listCameras()])
      .then(([equipmentNames, cameraNames]) => {
        setEquipments(equipmentNames)
        setCameras(cameraNames)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as configurações.'))
  }

  function loadProfiles() {
    listAllImportProfiles()
      .then(setProfiles)
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis de importação.'))
  }

  useEffect(() => {
    loadCodes()
    loadProfiles()
    listAnnotationProfiles()
      .then(setAnnotationProfiles)
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis de anotação.'))
  }, [])

  const annotationProfile = annotationProfiles.find((p) => p.name === annotationProfileName)
  const imageTargets = annotationProfile?.fields ?? []
  const objectTargets = (annotationProfile?.objects ?? []).filter((o) => !o.box)
  // Editando um perfil do mesmo perfil de anotação: as regras que a tela não
  // mostra (map, split, valor fixo) são mantidas ao salvar.
  const sameTarget = (editingOriginal?.annotation_profile || DEFAULT_ANNOTATION_PROFILE) === annotationProfileName
  const originalFields = sameTarget ? editingOriginal?.fields : undefined
  const originalObjects = sameTarget ? editingOriginal?.objects : undefined

  function objectSetting(type: string): ObjectSetting {
    return objectSettings[type] ?? originalObjects?.[type] ?? { mode: profileType === 'jpeg-ini' ? 'single' : 'row' }
  }

  function dateSetting(path: string): DateSetting {
    return dateSettings[path] ?? newDateSetting()
  }

  function changeDate(path: string, patch: Partial<DateSetting>) {
    setDateSettings((current) => ({ ...current, [path]: { ...(current[path] ?? newDateSetting()), ...patch } }))
  }

  // Trocar o perfil de anotação troca os campos de destino: o mapeamento
  // começa de novo.
  function changeAnnotationProfile(name: string) {
    setAnnotationProfileName(name)
    setFieldColumns({})
    setObjectColumns({})
    setObjectSettings({})
    setDateSettings({})
  }

  async function submit(type: 'equipment' | 'camera') {
    const name = (type === 'equipment' ? equipmentName : cameraName).trim()
    if (!name) {
      setError(`Informe o nome do ${type === 'equipment' ? 'equipamento' : 'câmera'}.`)
      return
    }

    setSaving(type)
    setError('')
    setMessage('')
    try {
      if (type === 'equipment') {
        await createEquipment(name)
        setEquipmentName('')
      } else {
        await createCamera(name)
        setCameraName('')
      }
      setMessage(`${type === 'equipment' ? 'Equipamento' : 'Câmera'} cadastrado com sucesso.`)
      loadCodes()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível concluir o cadastro.')
    } finally {
      setSaving('')
    }
  }

  function startRenameRegistry(type: 'equipment' | 'camera', name: string) {
    setEditingRegistry({ type, name, newName: name })
    setError('')
    setMessage('')
  }

  async function confirmRenameRegistry() {
    if (!editingRegistry) return
    const newName = editingRegistry.newName.trim()
    if (!newName || newName === editingRegistry.name) {
      setEditingRegistry(null)
      return
    }
    setSaving('registry')
    setError('')
    setMessage('')
    try {
      if (editingRegistry.type === 'equipment') {
        await renameEquipment(editingRegistry.name, newName)
      } else {
        await renameCamera(editingRegistry.name, newName)
      }
      setMessage(`Renomeado de "${editingRegistry.name}" para "${newName}" com sucesso.`)
      setEditingRegistry(null)
      loadCodes()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível renomear.')
    } finally {
      setSaving('')
    }
  }

  async function handleDeleteRegistry(type: 'equipment' | 'camera', name: string) {
    const label = type === 'equipment' ? 'equipamento' : 'câmera'
    if (!window.confirm(`Tem certeza que deseja apagar o ${label} "${name}"? Se estiver em uso por imagens, será desativado.`)) return
    setSaving('registry')
    setError('')
    setMessage('')
    try {
      const result = type === 'equipment'
        ? await deleteEquipment(name)
        : await deleteCamera(name)
      if (result.deactivated) {
        setMessage(`${type === 'equipment' ? 'Equipamento' : 'Câmera'} "${name}" desativado (usado por ${result.images} imagem(ns)).`)
      } else {
        setMessage(`${type === 'equipment' ? 'Equipamento' : 'Câmera'} "${name}" apagado com sucesso.`)
      }
      loadCodes()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar.')
    } finally {
      setSaving('')
    }
  }

  async function inspectProfileFile() {
    const isIni = profileType === 'jpeg-ini'
    if (isIni ? profileImages.length === 0 : !profileFile) {
      setError(isIni ? 'Selecione algumas imagens com INI para analisar.' : 'Selecione um arquivo CSV para analisar.')
      return
    }

    setSaving('inspect')
    setError('')
    setMessage('')
    try {
      const result = isIni ? await inspectIni(profileImages) : await inspectCsv(profileFile as File, delimiter, encoding)
      setInspection(result)
      // Editando: mantém as colunas que o perfil já usava, mesmo que estas
      // imagens/este CSV não tenham alguma.
      setAvailableColumns((current) => Array.from(new Set([...result.columns, ...(editingProfile ? current : [])])))
      if (!isIni && (!matchColumn || !result.columns.includes(matchColumn))) {
        setMatchColumn(result.columns[0] ?? '')
      }
      setMessage(isIni
        ? `${result.rows} imagem(ns) com INI, ${result.columns.length} chave(s). Configure o perfil abaixo.`
        : `${result.rows} linha(s) analisada(s). Configure o perfil abaixo.`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível analisar o CSV.')
    } finally {
      setSaving('')
    }
  }

  function handleEditProfile(profile: ImportProfile) {
    setEditingProfile(profile.name)
    setEditingOriginal(profile)
    setProfileType(profile.reader.type)
    setProfileImages([])
    setProfileName(profile.name)
    setProfileDescription(profile.description ?? '')
    setAnnotationProfileName(profile.annotation_profile || DEFAULT_ANNOTATION_PROFILE)
    const d = profile.reader.delimiter
    setDelimiter(d === ';' ? 'semicolon' : d === '\t' ? 'tab' : d === '|' ? 'pipe' : 'comma')
    setEncoding(profile.reader.encoding ?? 'utf-8')
    setMatchColumn(profile.match?.column ?? '')
    setMatchBy(profile.match?.by ?? 'original_name')

    const cols = new Set<string>()
    if (profile.match?.column) cols.add(profile.match.column)
    const dates: Record<string, DateSetting> = {}
    setFieldColumns(readRules(profile.fields, '', cols, dates))
    const columns: Record<string, Record<string, string>> = {}
    const settings: Record<string, ObjectSetting> = {}
    for (const [type, rules] of Object.entries(profile.objects ?? {})) {
      columns[type] = readRules(rules.fields, `${type}.`, cols, dates)
      settings[type] = { mode: rules.mode, max: rules.max, key: rules.key }
    }
    setObjectColumns(columns)
    setObjectSettings(settings)
    setDateSettings(dates)

    setAvailableColumns(Array.from(cols))
    setProfileFile(null)
    setInspection(null)
    setError('')
    setMessage(`Editando perfil "${profile.name}". Ajuste as configurações e salve.`)
    setActiveTab('profile_editor')
  }

  function handleCancelEdit() {
    setEditingProfile(null)
    setEditingOriginal(null)
    setProfileType('csv')
    setProfileImages([])
    setDateSettings({})
    setAnnotationProfileName(DEFAULT_ANNOTATION_PROFILE)
    setProfileName('')
    setProfileDescription('')
    setProfileFile(null)
    setInspection(null)
    setAvailableColumns([])
    setFieldColumns({})
    setObjectColumns({})
    setObjectSettings({})
    setError('')
    setMessage('')
  }

  async function handleDeleteProfile(name: string) {
    if (!window.confirm(`Tem certeza que deseja apagar o perfil "${name}"? Esta ação não pode ser desfeita.`)) {
      return
    }
    setSaving('profile')
    setError('')
    setMessage('')
    try {
      await deleteImportProfile(name)
      setMessage(`Perfil "${name}" apagado com sucesso.`)
      if (editingProfile === name) {
        handleCancelEdit()
      }
      loadProfiles()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível apagar o perfil.')
    } finally {
      setSaving('')
    }
  }

  async function submitProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalizedProfileName = profileName.trim().replace(/\s+/g, '-')
    const isIni = profileType === 'jpeg-ini'
    const mappedFields = mergeRules(originalFields, fieldColumns, imageTargets.map((f) => f.key), dateSettings)
    const mappedObjects: Record<string, ObjectRules> = {}
    for (const target of objectTargets) {
      const fields = mergeRules(originalObjects?.[target.key]?.fields, objectColumns[target.key] ?? {},
        target.fields.map((f) => f.key), dateSettings, `${target.key}.`)
      if (Object.keys(fields).length === 0) continue
      const setting = objectSetting(target.key)
      const mode = isIni && setting.mode === 'row' ? 'single' : setting.mode
      mappedObjects[target.key] = {
        mode,
        ...(mode === 'columns' ? { max: setting.max ?? 3 } : {}),
        ...(setting.key && fields[setting.key] ? { key: setting.key } : {}),
        fields,
      }
    }
    if (!normalizedProfileName || (!isIni && !matchColumn) || (Object.keys(mappedFields).length === 0 && Object.keys(mappedObjects).length === 0)) {
      setError(isIni
        ? 'Informe o nome e mapeie pelo menos um campo.'
        : 'Informe o nome, a coluna da imagem e mapeie pelo menos um campo.')
      return
    }
    if (!/^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$/.test(normalizedProfileName)) {
      setError('O nome do perfil deve usar apenas letras sem acento, números e hífens.')
      return
    }
    const badDate = Object.entries(dateSettings).find(([path, date]) => !date.format.trim() && (
      path.includes('.') ? objectColumns[path.split('.')[0]]?.[path.split('.')[1]] : fieldColumns[path]))
    if (badDate) {
      setError(`Informe o formato da data/hora de ${badDate[0]}.`)
      return
    }

    setSaving('profile')
    setError('')
    setMessage('')
    const profilePayload = {
      name: normalizedProfileName,
      description: profileDescription.trim() || undefined,
      ...(isIni ? {
        reader: { type: 'jpeg-ini' as const },
      } : {
        reader: { type: 'csv' as const, delimiter: delimiter === 'comma' ? undefined : delimiter === 'semicolon' ? ';' : delimiter === 'tab' ? '\t' : '|', encoding },
        match: { column: matchColumn, by: matchBy },
      }),
      annotation_profile: annotationProfileName,
      fields: mappedFields,
      ...(Object.keys(mappedObjects).length > 0 ? { objects: mappedObjects } : {}),
    }
    try {
      if (editingProfile) {
        // Nome mudou: renomeia primeiro (PATCH), depois troca o conteúdo (PUT).
        if (normalizedProfileName !== editingProfile) {
          await renameImportProfile(editingProfile, normalizedProfileName)
          setEditingProfile(normalizedProfileName)
        }
        await updateImportProfile(normalizedProfileName, profilePayload)
      } else {
        await createImportProfile(profilePayload)
      }
      // Limpa o formulário ANTES da mensagem (limpar também apaga a mensagem).
      handleCancelEdit()
      setMessage(`Perfil "${normalizedProfileName}" ${editingProfile ? 'atualizado' : 'criado'} com sucesso.`)
      setActiveTab('profiles_list')
      loadProfiles()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar o perfil.')
    } finally {
      setSaving('')
    }
  }

  // Um campo de destino: de qual coluna vem. No modo de colunas numeradas, o
  // nome é digitado (com {n}).
  function renderTarget(field: ProfileField, path: string, value: string, onChange: (column: string) => void, original: FieldRule | undefined, typed: boolean) {
    return (
      <label className="form-field" key={path}>
        <span>{fieldLabel(field)}{field.required ? ' *' : ''}</span>
        {typed ? (
          <input list="profile-columns" onChange={(event) => onChange(event.target.value.trim())} placeholder="Não mapear" value={value} />
        ) : (
          <select onChange={(event) => onChange(event.target.value)} value={value}>
            <option value="">Não mapear</option>
            {availableColumns.map((column) => <option key={column} value={column}>{column}</option>)}
            {value && !availableColumns.includes(value) && <option value={value}>{value}</option>}
          </select>
        )}
        {TYPE_HINTS[field.type] && <small className="profile-name-hint">{TYPE_HINTS[field.type]}</small>}
        {advancedLabel(original) && <small className="profile-name-hint">Mantido ao salvar: {advancedLabel(original)}</small>}
      </label>
    )
  }

  // O formato de um campo de data e hora mapeado.
  function renderDate(field: ProfileField, path: string, column: string) {
    const date = dateSetting(path)
    return (
      <div className="profile-form-grid" key={`date-${path}`}>
        <label className="form-field">
          <span>{fieldLabel(field)}: hora em outra {profileType === 'csv' ? 'coluna' : 'chave'} (opcional)</span>
          <select onChange={(event) => changeDate(path, { timeColumn: event.target.value })} value={date.timeColumn}>
            <option value="">A data já tem a hora</option>
            {availableColumns.filter((c) => c !== column).map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
          <small className="profile-name-hint">Junta as duas com um espaço: "data hora".</small>
        </label>
        <label className="form-field">
          <span>{fieldLabel(field)}: formato</span>
          <select
            onChange={(event) => (event.target.value === 'custom'
              ? changeDate(path, { custom: true })
              : changeDate(path, { custom: false, format: event.target.value }))}
            value={date.custom ? 'custom' : date.format}
          >
            {DATE_PRESETS.map(([layout, label]) => <option key={layout} value={layout}>{label}</option>)}
            <option value="custom">Outro formato…</option>
          </select>
          {date.custom && (
            <input onChange={(event) => changeDate(path, { format: event.target.value })} placeholder="2006-01-02 15:04:05" value={date.format} />
          )}
          <small className="profile-name-hint">
            {date.custom
              ? 'Escreva a data de referência 2006-01-02 15:04:05 no formato do CSV (não uma data do seu arquivo). Ex.: 02/01/2006 15:04:05.000'
              : `Layout: ${date.format}`}
            {inspection?.sample[0]?.[column] && ` · Exemplo: ${[
              inspection.sample[0][column],
              date.timeColumn ? inspection.sample[0][date.timeColumn] : '',
            ].filter(Boolean).join(' ')}`}
          </small>
        </label>
        <label className="form-field">
          <span>{fieldLabel(field)}: fuso horário</span>
          <input onChange={(event) => changeDate(path, { timezone: event.target.value })} value={date.timezone} />
        </label>
      </div>
    )
  }

  function renderRegistryList(type: 'equipment' | 'camera', items: string[]) {
    const label = type === 'equipment' ? 'equipamento' : 'câmera'
    if (items.length === 0) {
      return <p className="profile-empty-hint">Nenhum {label} cadastrado até o momento.</p>
    }
    return (
      <ul className="settings-list settings-list--actions">
        {items.map((item) => {
          const isEditing = editingRegistry?.type === type && editingRegistry.name === item
          return (
            <li key={item} className="settings-list__item">
              {isEditing ? (
                <div className="settings-list__rename">
                  <input
                    autoFocus
                    className="settings-list__rename-input"
                    onChange={(e) => setEditingRegistry((prev) => prev ? { ...prev, newName: e.target.value } : null)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') { e.preventDefault(); void confirmRenameRegistry() }
                      if (e.key === 'Escape') setEditingRegistry(null)
                    }}
                    value={editingRegistry.newName}
                  />
                  <button
                    className="button button--primary button--sm"
                    disabled={saving === 'registry'}
                    onClick={() => void confirmRenameRegistry()}
                    type="button"
                  >
                    Salvar
                  </button>
                  <button
                    className="button button--secondary button--sm"
                    onClick={() => setEditingRegistry(null)}
                    type="button"
                  >
                    Cancelar
                  </button>
                </div>
              ) : (
                <>
                  <span className="settings-list__name">{item}</span>
                  <div className="settings-list__actions">
                    <button
                      className="button button--secondary button--sm"
                      disabled={saving !== ''}
                      onClick={() => startRenameRegistry(type, item)}
                      type="button"
                    >
                      Renomear
                    </button>
                    <button
                      className="button button--danger button--sm"
                      disabled={saving !== ''}
                      onClick={() => void handleDeleteRegistry(type, item)}
                      type="button"
                    >
                      Apagar
                    </button>
                  </div>
                </>
              )}
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <section className="package-page settings-page">
      <button className="back-button" onClick={() => navigate('/pacotes')} type="button">← Voltar para pacotes</button>
      <div className="page-heading">
        <div>
          <h1>Configurar</h1>
          <p>Cadastre os equipamentos, câmeras, perfis de anotação e perfis de importação.</p>
        </div>
      </div>
      {error && <p className="form-error">{error}</p>}
      {message && <p className="settings-message">{message}</p>}

      <div className="settings-layout">
        <aside className="settings-sidebar">
          <nav className="settings-nav">
            <span className="settings-nav-section-title">Geral</span>
            <button
              className={`settings-nav-item ${activeTab === 'equipments' ? 'settings-nav-item--active' : ''}`}
              onClick={() => setActiveTab('equipments')}
              type="button"
            >
              <span className="settings-nav-text">Equipamentos</span>
              <span className="settings-nav-badge">{equipments.length}</span>
            </button>
            <button
              className={`settings-nav-item ${activeTab === 'cameras' ? 'settings-nav-item--active' : ''}`}
              onClick={() => setActiveTab('cameras')}
              type="button"
            >
              <span className="settings-nav-text">Câmeras</span>
              <span className="settings-nav-badge">{cameras.length}</span>
            </button>

            <div className="settings-nav-divider" />
            <span className="settings-nav-section-title">Anotação</span>
            <button
              className={`settings-nav-item ${activeTab === 'annotation_profiles' ? 'settings-nav-item--active' : ''}`}
              onClick={() => setActiveTab('annotation_profiles')}
              type="button"
            >
              <span className="settings-nav-text">Perfis de anotação</span>
            </button>

            <div className="settings-nav-divider" />
            <span className="settings-nav-section-title">Perfis de import</span>
            <button
              className={`settings-nav-item ${activeTab === 'profiles_list' ? 'settings-nav-item--active' : ''}`}
              onClick={() => setActiveTab('profiles_list')}
              type="button"
            >
              <span className="settings-nav-text">Perfis cadastrados</span>
              <span className="settings-nav-badge">{profiles.length}</span>
            </button>
            <button
              className={`settings-nav-item ${activeTab === 'profile_editor' ? 'settings-nav-item--active' : ''}`}
              onClick={() => setActiveTab('profile_editor')}
              type="button"
            >
              <span className="settings-nav-text">{editingProfile ? 'Editar perfil' : 'Criar novo perfil'}</span>
            </button>
          </nav>
        </aside>

        <main className="settings-content">
          {activeTab === 'equipments' && (
            <form className="create-card settings-card" onSubmit={(event) => { event.preventDefault(); void submit('equipment') }}>
              <h2>Equipamentos</h2>
              <p>Cadastre os equipamentos usados nas anotações (ex.: radar_01, camera_norte).</p>
              <label className="form-field">
                <span>Nome do equipamento</span>
                <input onChange={(event) => setEquipmentName(event.target.value)} placeholder="Digite o nome" value={equipmentName} />
              </label>
              <button className="button button--primary" disabled={saving === 'equipment'} type="submit">{saving === 'equipment' ? 'Salvando...' : 'Cadastrar equipamento'}</button>
              {renderRegistryList('equipment', equipments)}
            </form>
          )}

          {activeTab === 'cameras' && (
            <form className="create-card settings-card" onSubmit={(event) => { event.preventDefault(); void submit('camera') }}>
              <h2>Câmeras</h2>
              <p>Cadastre as câmeras disponíveis para anotação.</p>
              <label className="form-field">
                <span>Nome da câmera</span>
                <input onChange={(event) => setCameraName(event.target.value)} placeholder="Digite o nome" value={cameraName} />
              </label>
              <button className="button button--primary" disabled={saving === 'camera'} type="submit">{saving === 'camera' ? 'Salvando...' : 'Cadastrar câmera'}</button>
              {renderRegistryList('camera', cameras)}
            </form>
          )}

          {activeTab === 'annotation_profiles' && <AnnotationProfilesSection />}

          {activeTab === 'profiles_list' && (
            <div className="create-card settings-card">
              <div className="profile-header-flex">
                <div>
                  <h2>Perfis de import cadastrados</h2>
                  <p>Perfis que preenchem os rascunhos a partir de um CSV ou do INI embutido nas imagens.</p>
                </div>
                <button
                  className="button button--primary button--sm"
                  onClick={() => {
                    handleCancelEdit()
                    setActiveTab('profile_editor')
                  }}
                  type="button"
                >
                  + Novo perfil
                </button>
              </div>
              {profiles.length === 0 ? (
                <p className="profile-empty-hint">Nenhum perfil cadastrado até o momento.</p>
              ) : (
                <div className="profile-items-list">
                  {profiles.map((p) => {
                    const delimLabel = p.reader.delimiter === ';' ? 'Ponto e vírgula (;)' : p.reader.delimiter === '\t' ? 'Tabulação' : p.reader.delimiter === '|' ? 'Pipe (|)' : 'Vírgula (,)'
                    const isSelected = editingProfile === p.name
                    return (
                      <div className={`profile-item ${isSelected ? 'profile-item--selected' : ''}`} key={p.name}>
                        <div className="profile-item__header">
                          <div>
                            <strong className="profile-item__name">
                              {p.name}
                              <span className="profile-type-badge">{p.reader.type === 'jpeg-ini' ? 'INI' : 'CSV'}</span>
                            </strong>
                            {p.description && <span className="profile-item__desc">{p.description}</span>}
                          </div>
                          <div className="profile-item__actions">
                            <button
                              className="button button--secondary button--sm"
                              disabled={saving !== ''}
                              onClick={() => handleEditProfile(p)}
                              type="button"
                            >
                              Editar
                            </button>
                            <button
                              className="button button--danger button--sm"
                              disabled={saving !== ''}
                              onClick={() => void handleDeleteProfile(p.name)}
                              type="button"
                            >
                              Apagar
                            </button>
                          </div>
                        </div>
                        {p.reader.type === 'jpeg-ini' ? (
                          <div className="profile-item__details">
                            <span>Lê o INI gravado dentro de cada imagem</span>
                            <span><strong>Campos:</strong> {Object.keys(p.fields ?? {}).length + Object.values(p.objects ?? {}).reduce((n, o) => n + Object.keys(o.fields).length, 0)}</span>
                            <span><strong>Perfil de anotação:</strong> {p.annotation_profile || DEFAULT_ANNOTATION_PROFILE}</span>
                          </div>
                        ) : (
                          <div className="profile-item__details">
                            <span><strong>Separador:</strong> {delimLabel}</span>
                            <span><strong>Codificação:</strong> {p.reader.encoding ?? 'utf-8'}</span>
                            {p.match?.column && <span><strong>Coluna chave:</strong> {p.match.column} ({p.match.by === 'sha1' ? 'SHA1' : 'Nome original'})</span>}
                            <span><strong>Perfil de anotação:</strong> {p.annotation_profile || DEFAULT_ANNOTATION_PROFILE}</span>
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )}

          {activeTab === 'profile_editor' && (
            <form className="create-card settings-card" onSubmit={(event) => void submitProfile(event)}>
              <div className="profile-header-flex">
                <div>
                  <h2>{editingProfile ? `Editar perfil: ${editingProfile}` : `Criar novo perfil ${profileType === 'jpeg-ini' ? 'de INI' : 'CSV'}`}</h2>
                  <p>
                    {profileType === 'jpeg-ini'
                      ? 'Relacione as chaves do INI gravado dentro das imagens (Secao.Chave) aos campos de anotação.'
                      : 'Relacione as colunas do seu CSV aos campos de anotação.'}
                  </p>
                </div>
                {editingProfile && (
                  <button className="button button--secondary button--sm" onClick={handleCancelEdit} type="button">
                    Cancelar edição
                  </button>
                )}
              </div>
              {!editingProfile && (
                <div className="import-source profile-type-toggle" role="radiogroup" aria-label="Tipo do perfil">
                  {(['csv', 'jpeg-ini'] as const).map((type) => (
                    <button
                      aria-checked={profileType === type}
                      className={profileType === type ? 'import-source__option import-source__option--active' : 'import-source__option'}
                      key={type}
                      onClick={() => {
                        setProfileType(type)
                        setInspection(null)
                        setAvailableColumns([])
                        setFieldColumns({})
                        setObjectColumns({})
                        setObjectSettings({})
                        setDateSettings({})
                      }}
                      role="radio"
                      type="button"
                    >
                      {type === 'csv' ? 'CSV' : 'INI embutido na imagem'}
                    </button>
                  ))}
                </div>
              )}
              <div className="profile-form-grid">
                <label className="form-field">
                  <span>Nome do perfil</span>
                  <input onChange={(event) => setProfileName(event.target.value)} placeholder="Ex.: csv-rodovia" value={profileName} />
                  <small className="profile-name-hint">Use letras, números e hífens. Espaços viram hífens.</small>
                </label>
                <label className="form-field">
                  <span>Descrição (opcional)</span>
                  <input onChange={(event) => setProfileDescription(event.target.value)} placeholder="Formato do CSV da rodovia" value={profileDescription} />
                </label>
                <label className="form-field">
                  <span>Perfil de anotação</span>
                  <select onChange={(event) => changeAnnotationProfile(event.target.value)} value={annotationProfileName}>
                    {annotationProfiles.map((p) => <option key={p.name} value={p.name}>{p.name}{p.description ? ` — ${p.description}` : ''}</option>)}
                    {!annotationProfile && <option value={annotationProfileName}>{annotationProfileName} (não existe mais)</option>}
                  </select>
                  <small className="profile-name-hint">Os campos de destino são os deste perfil. Use o perfil de import nos pacotes que usam ele.</small>
                </label>
                {profileType === 'csv' && (
                  <>
                <label className="form-field">
                  <span>Separador</span>
                  <select onChange={(event) => setDelimiter(event.target.value)} value={delimiter}>
                    <option value="comma">Vírgula (,)</option>
                    <option value="semicolon">Ponto e vírgula (;)</option>
                    <option value="tab">Tabulação</option>
                    <option value="pipe">Barra vertical (|)</option>
                  </select>
                </label>
                <label className="form-field">
                  <span>Codificação</span>
                  <select onChange={(event) => setEncoding(event.target.value)} value={encoding}>
                    <option value="utf-8">UTF-8</option>
                    <option value="latin1">Latin-1</option>
                  </select>
                </label>
                  </>
                )}
              </div>
              {profileType === 'csv' ? (
                <label className="profile-file-picker">
                  <span>{profileFile?.name ?? (editingProfile ? 'Selecione um novo CSV para inspecionar outras colunas (opcional)' : 'Selecione o CSV para analisar')}</span>
                  <input accept=".csv,text/csv" onChange={(event) => {
                    setProfileFile(event.target.files?.[0] ?? null)
                    setInspection(null)
                    setError('')
                  }} type="file" />
                </label>
              ) : (
                <label className="profile-file-picker">
                  <span>
                    {profileImages.length > 0
                      ? `${profileImages.length} imagem(ns) selecionada(s)`
                      : 'Selecione algumas imagens de exemplo (até 20, podem ser criptografadas) para ler as chaves do INI'}
                  </span>
                  {/* sem "accept": as criptografadas não têm tipo de imagem, e o backend as abre */}
                  <input multiple onChange={(event) => {
                    setProfileImages(imageFiles(event.target.files ?? []).slice(0, 20))
                    setInspection(null)
                    setError('')
                  }} type="file" />
                </label>
              )}
              <button
                className="button button--secondary"
                disabled={(profileType === 'csv' ? !profileFile : profileImages.length === 0) || saving !== ''}
                onClick={() => void inspectProfileFile()}
                type="button"
              >
                {saving === 'inspect' ? 'Analisando...' : profileType === 'csv' ? 'Analisar CSV' : 'Ler o INI das imagens'}
              </button>
              {(inspection || availableColumns.length > 0) && (
                <div className="profile-editor">
                  <datalist id="profile-columns">
                    {availableColumns.map((column) => <option key={column} value={column} />)}
                  </datalist>
                  {inspection && (
                    <p className="profile-editor__summary">
                      {profileType === 'csv'
                        ? `${inspection.rows} linha(s), ${inspection.columns.length} coluna(s) encontrada(s).`
                        : `${inspection.rows} imagem(ns) com INI, ${inspection.columns.length} chave(s) encontrada(s).`}
                    </p>
                  )}
                  {profileType === 'csv' && (
                  <div className="profile-form-grid">
                    <label className="form-field">
                      <span>Coluna que identifica a imagem</span>
                      <select onChange={(event) => setMatchColumn(event.target.value)} value={matchColumn}>
                        {availableColumns.map((column) => <option key={column} value={column}>{column}</option>)}
                      </select>
                    </label>
                    <label className="form-field">
                      <span>Comparar com</span>
                      <select onChange={(event) => setMatchBy(event.target.value as 'original_name' | 'sha1')} value={matchBy}>
                        <option value="original_name">Nome original do arquivo</option>
                        <option value="sha1">SHA1 da imagem</option>
                      </select>
                    </label>
                  </div>
                  )}
                  <h3>Campos da imagem</h3>
                  <p className="profile-editor__hint">
                    Os campos do perfil de anotação {annotationProfileName}. Escolha "Não mapear" para os que não existem no {profileType === 'csv' ? 'CSV' : 'INI'}.
                  </p>
                  {imageTargets.length === 0 && <p className="profile-empty-hint">Este perfil não tem campos na imagem.</p>}
                  <div className="profile-fields">
                    {imageTargets.map((field) => renderTarget(field, field.key, fieldColumns[field.key] ?? '',
                      (column) => setFieldColumns((current) => ({ ...current, [field.key]: column })),
                      originalFields?.[field.key], false))}
                  </div>
                  {imageTargets.filter((field) => field.type === 'datetime' && fieldColumns[field.key]).map((field) => renderDate(field, field.key, fieldColumns[field.key]))}

                  {objectTargets.map((target) => {
                    const setting = objectSetting(target.key)
                    const columns = objectColumns[target.key] ?? {}
                    const mapped = target.fields.filter((field) => columns[field.key])
                    const setSetting = (patch: Partial<ObjectSetting>) =>
                      setObjectSettings((current) => ({ ...current, [target.key]: { ...setting, ...patch } }))
                    return (
                      <div className="profile-object" key={target.key}>
                        <h3>{fieldLabel(target)}</h3>
                        <div className="profile-form-grid">
                          <label className="form-field">
                            <span>Como os objetos aparecem</span>
                            {/* no CSV, "single" e "row" são o mesmo: um objeto por linha */}
                            <select
                              onChange={(event) => setSetting({ mode: event.target.value as ObjectSetting['mode'] })}
                              value={setting.mode === 'columns' ? 'columns' : profileType === 'csv' ? 'row' : 'single'}
                            >
                              {profileType === 'csv'
                                ? <option value="row">Um por linha (linhas da mesma imagem são agrupadas)</option>
                                : <option value="single">Um por imagem</option>}
                              <option value="columns">Colunas numeradas (Placa1, Placa2…)</option>
                            </select>
                          </label>
                          {setting.mode === 'columns' && (
                            <label className="form-field">
                              <span>Quantos, no máximo</span>
                              <input max={20} min={1} onChange={(event) => setSetting({ max: Number.parseInt(event.target.value, 10) || 1 })} type="number" value={setting.max ?? 3} />
                            </label>
                          )}
                          <label className="form-field">
                            <span>O objeto existe quando…</span>
                            <select onChange={(event) => setSetting({ key: event.target.value || undefined })} value={setting.key && columns[setting.key] ? setting.key : ''}>
                              <option value="">algum campo tem valor</option>
                              {mapped.map((field) => <option key={field.key} value={field.key}>{fieldLabel(field)} tem valor</option>)}
                            </select>
                          </label>
                        </div>
                        {setting.mode === 'columns' && (
                          <p className="profile-editor__hint">Escreva o nome da coluna com {'{n}'} no lugar do número, ex.: Placa{'{n}'}.</p>
                        )}
                        <div className="profile-fields">
                          {target.fields.map((field) => renderTarget(field, `${target.key}.${field.key}`, columns[field.key] ?? '',
                            (column) => setObjectColumns((current) => ({ ...current, [target.key]: { ...(current[target.key] ?? {}), [field.key]: column } })),
                            originalObjects?.[target.key]?.fields[field.key], setting.mode === 'columns'))}
                        </div>
                        {target.fields.filter((field) => field.type === 'datetime' && columns[field.key]).map((field) => renderDate(field, `${target.key}.${field.key}`, columns[field.key]))}
                      </div>
                    )
                  })}
                  <div className="profile-submit-row">
                    <button className="button button--primary" disabled={saving !== ''} type="submit">
                      {saving === 'profile' ? 'Salvando...' : editingProfile ? 'Salvar alterações' : 'Criar perfil CSV'}
                    </button>
                    {editingProfile && (
                      <button className="button button--secondary" onClick={handleCancelEdit} type="button">
                        Cancelar
                      </button>
                    )}
                  </div>
                </div>
              )}
            </form>
          )}
        </main>
      </div>
    </section>
  )
}
