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
  getCodes,
  inspectCsv,
  listImportProfiles,
  renameCamera,
  renameEquipment,
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

type EditingRegistry = { type: 'equipment' | 'camera'; name: string; newName: string } | null

export function SettingsPage() {
  const navigate = useNavigate()
  const [equipments, setEquipments] = useState<string[]>([])
  const [cameras, setCameras] = useState<string[]>([])
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [editingProfile, setEditingProfile] = useState<string | null>(null)
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
    getCodes()
      .then((codes) => {
        setEquipments(codes.equipments)
        setCameras(codes.cameras)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as configurações.'))
  }

  function loadProfiles() {
    listImportProfiles()
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
    if (!profileFile) {
      setError('Selecione um arquivo CSV para analisar.')
      return
    }

    setSaving('inspect')
    setError('')
    setMessage('')
    try {
      const result = await inspectCsv(profileFile, delimiter, encoding)
      setInspection(result)
      setAvailableColumns(result.columns)
      if (!matchColumn || !result.columns.includes(matchColumn)) {
        setMatchColumn(result.columns[0] ?? '')
      }
      setMessage(`${result.rows} linha(s) analisada(s). Configure o perfil abaixo.`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível analisar o CSV.')
    } finally {
      setSaving('')
    }
  }

  function handleEditProfile(profile: ImportProfile) {
    setEditingProfile(profile.name)
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
        if (field === 'captured_at') {
          if (rule.date_format) setDateFormat(rule.date_format)
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
    const mappedFields: Record<string, FieldRule> = {}
    const mappedVehicleFields: Record<string, FieldRule> = {}
    Object.entries(fieldColumns)
      .filter(([, column]) => column)
      .forEach(([field, column]) => {
        mappedFields[field] = { column }
      })
    Object.entries(vehicleFieldColumns)
      .filter(([, column]) => column)
      .forEach(([field, column]) => {
        mappedVehicleFields[field] = { column }
      })
    if (!normalizedProfileName || !matchColumn || Object.keys(mappedFields).length === 0) {
      setError('Informe o nome, a coluna da imagem e mapeie pelo menos um campo.')
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
    if (mappedFields.captured_at) {
      mappedFields.captured_at = { column: mappedFields.captured_at.column, date_format: dateFormat, timezone }
    }

    setSaving('profile')
    setError('')
    setMessage('')
    const profilePayload = {
      name: normalizedProfileName,
      description: profileDescription.trim() || undefined,
      reader: { type: 'csv' as const, delimiter: delimiter === 'comma' ? undefined : delimiter === 'semicolon' ? ';' : delimiter === 'tab' ? '\t' : '|' , encoding },
      match: { column: matchColumn, by: matchBy },
      fields: mappedFields,
      ...(mappedVehicleFields.plate ? { vehicles: { mode: 'row' as const, fields: mappedVehicleFields } } : {}),
    }
    try {
      if (editingProfile) {
        await updateImportProfile(editingProfile, profilePayload)
        setMessage(`Perfil "${normalizedProfileName}" atualizado com sucesso.`)
      } else {
        await createImportProfile(profilePayload)
        setMessage(`Perfil "${normalizedProfileName}" criado com sucesso.`)
      }
      handleCancelEdit()
      loadProfiles()
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível salvar o perfil CSV.')
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
            <span className="settings-nav-section-title">CSV</span>
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
                  <h2>Perfis de importação CSV cadastrados</h2>
                  <p>Perfis disponíveis para pré-preenchimento ao importar CSV nos pacotes.</p>
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
                            <strong className="profile-item__name">{p.name}</strong>
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
                        <div className="profile-item__details">
                          <span><strong>Separador:</strong> {delimLabel}</span>
                          <span><strong>Codificação:</strong> {p.reader.encoding ?? 'utf-8'}</span>
                          {p.match?.column && <span><strong>Coluna chave:</strong> {p.match.column} ({p.match.by === 'sha1' ? 'SHA1' : 'Nome original'})</span>}
                        </div>
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
                  <h2>{editingProfile ? `Editar perfil: ${editingProfile}` : 'Criar novo perfil CSV'}</h2>
                  <p>{editingProfile ? 'Modifique os parâmetros do perfil e clique em Salvar alterações.' : 'Crie um perfil para relacionar as colunas do seu CSV aos campos de anotação.'}</p>
                </div>
                {editingProfile && (
                  <button className="button button--secondary button--sm" onClick={handleCancelEdit} type="button">
                    Cancelar edição
                  </button>
                )}
              </div>
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
              </div>
              <label className="profile-file-picker">
                <span>{profileFile?.name ?? (editingProfile ? 'Selecione um novo CSV para inspecionar outras colunas (opcional)' : 'Selecione o CSV para analisar')}</span>
                <input accept=".csv,text/csv" onChange={(event) => {
                  setProfileFile(event.target.files?.[0] ?? null)
                  setInspection(null)
                  setError('')
                }} type="file" />
              </label>
              <button className="button button--secondary" disabled={!profileFile || saving !== ''} onClick={() => void inspectProfileFile()} type="button">
                {saving === 'inspect' ? 'Analisando...' : 'Analisar CSV'}
              </button>
              {(inspection || availableColumns.length > 0) && (
                <div className="profile-editor">
                  {inspection && (
                    <p className="profile-editor__summary">{inspection.rows} linha(s), {inspection.columns.length} coluna(s) encontrada(s).</p>
                  )}
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
                  <h3>Mapeamento dos campos</h3>
                  <p className="profile-editor__hint">Escolha "Não mapear" para os campos que não existem no CSV.</p>
                  <div className="profile-fields">
                    {profileFields.map(([field, label]) => (
                      <label className="form-field" key={field}>
                        <span>{label}</span>
                        <select onChange={(event) => setFieldColumns((current) => ({ ...current, [field]: event.target.value }))} value={fieldColumns[field] ?? ''}>
                          <option value="">Não mapear</option>
                          {availableColumns.map((column) => <option key={column} value={column}>{column}</option>)}
                        </select>
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
                      </label>
                    ))}
                  </div>
                  {fieldColumns.captured_at && (
                    <div className="profile-form-grid">
                      <label className="form-field">
                        <span>Formato da data/hora</span>
                        <input onChange={(event) => setDateFormat(event.target.value)} value={dateFormat} />
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
