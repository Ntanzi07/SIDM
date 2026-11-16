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
  listCameras,
  listEquipments,
  renameCamera,
  renameEquipment,
  renameImportProfile,
  updateImportProfile,
  type CsvInspection,
  type FieldRule,
  type ImportProfile,
} from '../../services/api'
import './Settings.css'

const profileFields = [
  ['equipment', 'Equipamento'],
  ['camera', 'Câmera'],
  ['direction', 'Direção'],
  ['resolution', 'Resolução'],
  ['capture_angle', 'Ângulo de captura'],
  ['day_time', 'Período do dia'],
  ['image_range', 'Alcance da imagem'],
  ['weather', 'Clima'],
  ['conditions', 'Condições'],
  ['captured_at', 'Data e hora'],
] as const

const vehicleFields = [
  ['plate', 'Placa'],
  ['plate_type', 'Tipo da placa'],
  ['plate_category', 'Classificação da placa'],
  ['type', 'Tipo do veículo'],
  ['color', 'Cor do veículo'],
] as const

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
    out[field] = { ...previous, column }
  }
  return out
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
  // Data e hora em colunas separadas (comum no INI: Data= e Hora=).
  const [timeColumn, setTimeColumn] = useState('')
  const [dateCustom, setDateCustom] = useState(false)
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
  const [vehicleFieldColumns, setVehicleFieldColumns] = useState<Record<string, string>>({})
  const [dateFormat, setDateFormat] = useState('02/01/2006 15:04:05')
  const [timezone, setTimezone] = useState('America/Sao_Paulo')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [saving, setSaving] = useState<'equipment' | 'camera' | 'inspect' | 'profile' | 'registry' | ''>('')
  const [activeTab, setActiveTab] = useState<'equipments' | 'cameras' | 'profiles_list' | 'profile_editor'>('equipments')

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
  }, [])

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
    setTimeColumn('')
    setProfileName(profile.name)
    setProfileDescription(profile.description ?? '')
    const d = profile.reader.delimiter
    setDelimiter(d === ';' ? 'semicolon' : d === '\t' ? 'tab' : d === '|' ? 'pipe' : 'comma')
    setEncoding(profile.reader.encoding ?? 'utf-8')
    setMatchColumn(profile.match?.column ?? '')
    setMatchBy(profile.match?.by ?? 'original_name')

    const cols = new Set<string>()
    if (profile.match?.column) cols.add(profile.match.column)

    const initialFieldColumns: Record<string, string> = {}
    if (profile.fields) {
      Object.entries(profile.fields).forEach(([field, rule]) => {
        if (rule.column) {
          initialFieldColumns[field] = rule.column
          cols.add(rule.column)
        }
        // Data e hora separadas ficam num template "{Data} {Hora}".
        const parts = field === 'captured_at' ? rule.template?.match(/^\{([^{}]+)\} \{([^{}]+)\}$/) : null
        if (parts) {
          initialFieldColumns[field] = parts[1]
          setTimeColumn(parts[2])
          cols.add(parts[1])
          cols.add(parts[2])
        }
        if (field === 'captured_at') {
          if (rule.date_format) {
            setDateFormat(rule.date_format)
            setDateCustom(!DATE_PRESETS.some(([layout]) => layout === rule.date_format))
          }
          if (rule.timezone) setTimezone(rule.timezone)
        }
      })
    }
    setFieldColumns(initialFieldColumns)

    const initialVehicleColumns: Record<string, string> = {}
    if (profile.vehicles?.fields) {
      Object.entries(profile.vehicles.fields).forEach(([field, rule]) => {
        if (rule.column) {
          initialVehicleColumns[field] = rule.column
          cols.add(rule.column)
        }
      })
    }
    setVehicleFieldColumns(initialVehicleColumns)

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
    setTimeColumn('')
    setDateFormat(DATE_PRESETS[0][0])
    setDateCustom(false)
    setProfileName('')
    setProfileDescription('')
    setProfileFile(null)
    setInspection(null)
    setAvailableColumns([])
    setFieldColumns({})
    setVehicleFieldColumns({})
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
    const mappedFields = mergeRules(editingOriginal?.fields, fieldColumns, profileFields.map(([field]) => field))
    const mappedVehicleFields = mergeRules(editingOriginal?.vehicles?.fields, vehicleFieldColumns, vehicleFields.map(([field]) => field))
    const isIni = profileType === 'jpeg-ini'
    if (!normalizedProfileName || (!isIni && !matchColumn) || (Object.keys(mappedFields).length === 0 && !mappedVehicleFields.plate)) {
      setError(isIni
        ? 'Informe o nome e mapeie pelo menos um campo.'
        : 'Informe o nome, a coluna da imagem e mapeie pelo menos um campo.')
      return
    }
    if (!/^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$/.test(normalizedProfileName)) {
      setError('O nome do perfil deve usar apenas letras sem acento, números e hífens.')
      return
    }
    if (Object.keys(mappedVehicleFields).length > 0 && !mappedVehicleFields.plate) {
      setError('Para mapear dados de veículo, a coluna Placa é obrigatória.')
      return
    }
    if (fieldColumns.captured_at) {
      if (!dateFormat.trim()) {
        setError('Informe o formato da data/hora.')
        return
      }
      // Uma coluna só (column) ou data + hora juntas (template "{Data} {Hora}").
      const base: FieldRule = { ...mappedFields.captured_at }
      delete base.column
      delete base.template
      delete base.value
      mappedFields.captured_at = {
        ...base,
        ...(timeColumn ? { template: `{${fieldColumns.captured_at}} {${timeColumn}}` } : { column: fieldColumns.captured_at }),
        date_format: dateFormat.trim(),
        timezone: timezone.trim() || undefined,
      }
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
      fields: mappedFields,
      ...(mappedVehicleFields.plate ? {
        vehicles: {
          // INI: um veículo por imagem. CSV: o modo (e o max, no modo
          // "columns") vem do perfil original, se houver.
          mode: isIni ? 'single' as const : editingOriginal?.vehicles?.mode ?? 'row' as const,
          max: editingOriginal?.vehicles?.max,
          fields: mappedVehicleFields,
        },
      } : {}),
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
          <p>Cadastre os equipamentos, câmeras e perfis de importação.</p>
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
                            <span><strong>Campos:</strong> {Object.keys(p.fields ?? {}).length + Object.keys(p.vehicles?.fields ?? {}).length}</span>
                          </div>
                        ) : (
                          <div className="profile-item__details">
                            <span><strong>Separador:</strong> {delimLabel}</span>
                            <span><strong>Codificação:</strong> {p.reader.encoding ?? 'utf-8'}</span>
                            {p.match?.column && <span><strong>Coluna chave:</strong> {p.match.column} ({p.match.by === 'sha1' ? 'SHA1' : 'Nome original'})</span>}
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
                        setVehicleFieldColumns({})
                        setTimeColumn('')
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
                  <h3>Mapeamento dos campos</h3>
                  <p className="profile-editor__hint">Escolha "Não mapear" para os campos que não existem no {profileType === 'csv' ? 'CSV' : 'INI'}.</p>
                  <div className="profile-fields">
                    {profileFields.map(([field, label]) => (
                      <label className="form-field" key={field}>
                        <span>{label}</span>
                        <select onChange={(event) => setFieldColumns((current) => ({ ...current, [field]: event.target.value }))} value={fieldColumns[field] ?? ''}>
                          <option value="">Não mapear</option>
                          {availableColumns.map((column) => <option key={column} value={column}>{column}</option>)}
                        </select>
                        {advancedLabel(editingOriginal?.fields?.[field]) && (
                          <small className="profile-name-hint">Mantido ao salvar: {advancedLabel(editingOriginal?.fields?.[field])}</small>
                        )}
                      </label>
                    ))}
                  </div>
                  <h3>Dados do veículo</h3>
                  <p className="profile-editor__hint">A placa é obrigatória para importar os demais dados do veículo.</p>
                  <div className="profile-fields">
                    {vehicleFields.map(([field, label]) => (
                      <label className="form-field" key={field}>
                        <span>{label}</span>
                        <select onChange={(event) => setVehicleFieldColumns((current) => ({ ...current, [field]: event.target.value }))} value={vehicleFieldColumns[field] ?? ''}>
                          <option value="">Não mapear</option>
                          {availableColumns.map((column) => <option key={column} value={column}>{column}</option>)}
                        </select>
                        {advancedLabel(editingOriginal?.vehicles?.fields[field]) && (
                          <small className="profile-name-hint">Mantido ao salvar: {advancedLabel(editingOriginal?.vehicles?.fields[field])}</small>
                        )}
                      </label>
                    ))}
                  </div>
                  {fieldColumns.captured_at && (
                    <div className="profile-form-grid">
                      <label className="form-field">
                        <span>Hora em outra {profileType === 'csv' ? 'coluna' : 'chave'} (opcional)</span>
                        <select onChange={(event) => setTimeColumn(event.target.value)} value={timeColumn}>
                          <option value="">A data já tem a hora</option>
                          {availableColumns.filter((column) => column !== fieldColumns.captured_at).map((column) => <option key={column} value={column}>{column}</option>)}
                        </select>
                        <small className="profile-name-hint">Junta as duas com um espaço: "data hora".</small>
                      </label>
                      <label className="form-field">
                        <span>Formato da data/hora</span>
                        <select
                          onChange={(event) => {
                            if (event.target.value === 'custom') {
                              setDateCustom(true)
                            } else {
                              setDateCustom(false)
                              setDateFormat(event.target.value)
                            }
                          }}
                          value={dateCustom ? 'custom' : dateFormat}
                        >
                          {DATE_PRESETS.map(([layout, label]) => <option key={layout} value={layout}>{label}</option>)}
                          <option value="custom">Outro formato…</option>
                        </select>
                        {dateCustom && (
                          <input onChange={(event) => setDateFormat(event.target.value)} placeholder="2006-01-02 15:04:05" value={dateFormat} />
                        )}
                        <small className="profile-name-hint">
                          {dateCustom
                            ? 'Escreva a data de referência 2006-01-02 15:04:05 no formato do CSV (não uma data do seu arquivo). Ex.: 02/01/2006 15:04:05.000'
                            : `Layout: ${dateFormat}`}
                          {inspection?.sample[0]?.[fieldColumns.captured_at] && ` · Exemplo: ${[
                            inspection.sample[0][fieldColumns.captured_at],
                            timeColumn ? inspection.sample[0][timeColumn] : '',
                          ].filter(Boolean).join(' ')}`}
                        </small>
                      </label>
                      <label className="form-field">
                        <span>Fuso horário</span>
                        <input onChange={(event) => setTimezone(event.target.value)} value={timezone} />
                      </label>
                    </div>
                  )}
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
