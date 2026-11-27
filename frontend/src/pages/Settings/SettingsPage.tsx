import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import {
  createImportProfile,
  deleteImportProfile,
  imageFiles,
  inspectCsv,
  inspectIni,
  listAllImportProfiles,
  listAnnotationProfiles,
  listCameras,
  listEquipments,
  listInactiveCameras,
  listInactiveEquipments,
  renameImportProfile,
  updateImportProfile,
  type CsvInspection,
  type FieldRule,
  type ImportProfile,
  type ObjectRules,
} from '../../services/api'
import type { AnnotationProfile, ProfileField } from '../../types/annotation'
import { AnnotationProfilesSection } from './AnnotationProfilesSection'
import { ImportProfileList } from './ImportProfileList'
import { RegistrySection } from './RegistrySection'
import { SettingRow, SettingsGroup, SettingsHeader } from './SettingsUI'
import './ImportEditor.css'
import { SettingsIcon, type SettingsIconName } from './SettingsIcons'
import { ColumnPicker } from './ColumnPicker'
import { useToastState } from '../../hooks/useToast'
import { confirmDialog } from '../../services/confirm'
import { Bone, Skeleton } from '../../components/Skeleton/Skeleton'
import { useSearchParams } from 'react-router-dom'
import { useIsAdmin } from '../../services/user'
import { AccountSection } from './AccountSection'
import { ShortcutsSection } from './ShortcutsSection'
import { SystemSection } from './SystemSection'
import { UsersPage } from '../Users/UsersPage'
import './Settings.css'

// As telas do Configurar e o nome de cada uma na URL (?aba=atalhos), para
// abrir direto (o menu da conta leva para os atalhos e os usuários).
type Tab = 'account' | 'shortcuts' | 'equipments' | 'cameras' | 'annotation_profiles' | 'annotation_editor' | 'profiles_list' | 'profile_editor' | 'users' | 'system'
const TAB_SLUGS: Record<Tab, string> = {
  account: 'conta', shortcuts: 'atalhos', equipments: 'equipamentos', cameras: 'cameras',
  annotation_profiles: 'perfis-anotacao', annotation_editor: 'editor-anotacao',
  profiles_list: 'perfis-import', profile_editor: 'editor-import', users: 'usuarios', system: 'sistema',
}
const USER_TABS: Tab[] = ['account', 'shortcuts'] // o resto é de admin
// os nomes no seletor da tela estreita
const TAB_LABELS: Record<Tab, string> = {
  account: 'Conta', shortcuts: 'Atalhos de teclado', annotation_profiles: 'Perfis de anotação', annotation_editor: 'Editor de perfil de anotação',
  equipments: 'Equipamentos', cameras: 'Câmeras', profiles_list: 'Perfis de importação', profile_editor: 'Editor de perfil de importação',
  users: 'Usuários', system: 'Armazenamento',
}

const DEFAULT_ANNOTATION_PROFILE = 'veiculos'

// O que a tela guarda de um campo de data e hora: o formato, o fuso e,
// opcionalmente, a hora numa coluna separada (comum no INI: Data= e Hora=).
// format vazio = automático (o backend reconhece os formatos comuns).
type DateSetting = { format: string; custom: boolean; timezone: string; timeColumn: string; msColumn: string }
type ObjectSetting = { mode: ObjectRules['mode']; max?: number; key?: string }

// Formatos de data. O padrão é o automático (sem formato fixo: o backend
// reconhece dia/mês/ano, ano-mês-dia, hora com ou sem segundos...). Os
// outros usam o layout do Go: a data de referência 2006-01-02 15:04:05
// escrita do jeito que aparece no arquivo.
const DATE_PRESETS = [
  ['', 'Automático'],
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
      // Data, hora e milissegundo em partes (column, time_column, ms_column).
      delete previous.time_column
      delete previous.ms_column
      delete previous.date_format
      delete previous.timezone
      out[field] = {
        ...previous,
        column,
        ...(date.timeColumn ? { time_column: date.timeColumn } : {}),
        ...(date.msColumn ? { ms_column: date.msColumn } : {}),
        ...(date.format.trim() ? { date_format: date.format.trim() } : {}),
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
    if (rule.date_format || rule.time_column || rule.ms_column || rule.timezone) {
      const date = newDateSetting()
      date.format = rule.date_format ?? ''
      date.custom = !DATE_PRESETS.some(([layout]) => layout === date.format)
      if (rule.timezone) date.timezone = rule.timezone
      for (const [key, value] of [['timeColumn', rule.time_column], ['msColumn', rule.ms_column]] as const) {
        if (value) {
          date[key] = value
          cols.add(value)
        }
      }
      // Formato antigo: data e hora num template "{Data} {Hora}".
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
  return { format: '', custom: false, timezone: 'America/Sao_Paulo', timeColumn: '', msColumn: '' }
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

// A silhueta do Configurar enquanto carrega (só o admin espera: cadastros e
// perfis). O título é o de verdade; o menu e o painel são blocos, nos
// mesmos lugares (grupos do menu, linhas do painel).
const MENU_SHAPE = [[56, 118], [96, 84, 70], [112], [64, 104]] // nome de cada item, por grupo
const ROW_SHAPE = [[96, '62%'], [120, '48%'], [84, '70%'], [108, '54%']]

function SettingsSkeleton() {
  return (
    <section className="package-page settings-page">
      <div className="page-heading">
        <div>
          <h1>Configurar</h1>
          <p>Sua conta, o que se anota, como se importa e quem usa o SIDM.</p>
        </div>
      </div>
      <Skeleton label="Carregando as configurações">
        <div className="settings-skeleton">
          <aside className="settings-sidebar">
            <div className="settings-nav">
              {MENU_SHAPE.map((group, g) => (
                <div className="settings-skeleton__group" key={g}>
                  <Bone h={10} w={64} />
                  {group.map((w, i) => (
                    <div className="settings-skeleton__item" key={i}>
                      <Bone h={16} r={4} w={16} />
                      <Bone h={12} w={w} />
                    </div>
                  ))}
                </div>
              ))}
            </div>
          </aside>
          <div className="settings-content">
            <div className="set-header">
              <div className="settings-skeleton__title">
                <Bone h={24} w={220} />
                <Bone h={13} w="min(460px, 90%)" />
              </div>
            </div>
            <div className="set-group">
              <div className="set-group__head"><Bone h={14} w={120} /></div>
              <div className="set-group__panel">
                {ROW_SHAPE.map(([label, help], i) => (
                  <div className="set-row" key={i}>
                    <div className="set-row__text">
                      <Bone h={14} w={label} />
                      <Bone h={12} w={help} />
                    </div>
                    <Bone h={34} r={8} w={220} />
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </Skeleton>
    </section>
  )
}

export function SettingsPage() {
  const [equipments, setEquipments] = useState<string[]>([])
  const [cameras, setCameras] = useState<string[]>([])
  // Desativados: apagados em Configurar quando ainda havia imagens usando.
  const [inactiveEquipments, setInactiveEquipments] = useState<string[]>([])
  const [inactiveCameras, setInactiveCameras] = useState<string[]>([])
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [editingProfile, setEditingProfile] = useState<string | null>(null)
  const [editingOriginal, setEditingOriginal] = useState<ImportProfile | null>(null)
  // Vendo um perfil padrão (vem com o SIDM): o formulário fica só leitura.
  const [viewOnly, setViewOnly] = useState(false)
  // csv: os dados vêm de um CSV. jpeg-ini: do INI gravado dentro de cada JPEG.
  const [profileType, setProfileType] = useState<'csv' | 'jpeg-ini'>('csv')
  const [profileImages, setProfileImages] = useState<File[]>([])
  const [annotationProfiles, setAnnotationProfiles] = useState<AnnotationProfile[]>([])
  // O perfil de anotação que o perfil de import preenche (os campos de destino).
  const [annotationProfileName, setAnnotationProfileName] = useState(DEFAULT_ANNOTATION_PROFILE)
  // Formato de cada campo de data mapeado ("captured_at" ou "<tipo>.<campo>").
  const [dateSettings, setDateSettings] = useState<Record<string, DateSetting>>({})
  const [availableColumns, setAvailableColumns] = useState<string[]>([])
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
  const [, setError] = useToastState('error')
  const [, setMessage] = useToastState('success')
  const [saving, setSaving] = useState<'inspect' | 'profile' | ''>('')
  const admin = useIsAdmin()
  const [searchParams, setSearchParams] = useSearchParams()
  const fromUrl = (Object.keys(TAB_SLUGS) as Tab[]).find((tab) => TAB_SLUGS[tab] === searchParams.get('aba'))
  // a aba é a da URL: o menu da conta muda o ?aba= com a tela aberta
  const activeTab: Tab = fromUrl && (admin || USER_TABS.includes(fromUrl)) ? fromUrl : 'account'
  const setActiveTab = (tab: Tab) => setSearchParams({ aba: TAB_SLUGS[tab] }, { replace: true })
  // Quantidade e nome do editor dos perfis de anotação, para o menu.
  const [annotationSummary, setAnnotationSummary] = useState({ count: 0, editorLabel: 'Criar novo perfil' })

  const [loaded, setLoaded] = useState(!admin) // a primeira carga (usuário normal: nada a carregar)
  // O editor de import como abriu, para saber se mudou (e perguntar antes de
  // descartar). null = tirar a foto depois da próxima renderização, quando os
  // set* de abrir (editar, duplicar, novo) já valeram.
  const [importSnapshot, setImportSnapshot] = useState<string | null>(null)

  function loadCodes() {
    return Promise.all([listEquipments(), listCameras(), listInactiveEquipments(), listInactiveCameras()])
      .then(([equipmentNames, cameraNames, offEquipments, offCameras]) => {
        setEquipments(equipmentNames)
        setCameras(cameraNames)
        setInactiveEquipments(offEquipments)
        setInactiveCameras(offCameras)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar as configurações.'))
  }

  function loadProfiles() {
    return listAllImportProfiles()
      .then(setProfiles)
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis de importação.'))
  }

  useEffect(() => {
    if (!admin) return // usuário normal: só a própria conta (nada a carregar)
    void Promise.allSettled([
      loadCodes(),
      loadProfiles(),
      listAnnotationProfiles()
        .then(setAnnotationProfiles)
        .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis de anotação.')),
    ]).then(() => setLoaded(true))
    // só ao abrir a tela: loadCodes/loadProfiles usam setters estáveis
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
      // o resultado aparece embaixo do arquivo (sem balão repetindo)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível analisar o CSV.')
    } finally {
      setSaving('')
    }
  }

  function handleEditProfile(profile: ImportProfile) {
    setImportSnapshot(null) // a foto é tirada depois que tudo abaixo valer
    setViewOnly(false)
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
    setActiveTab('profile_editor')
  }

  // Perfil padrão: abre o mesmo formulário, só para ler.
  function handleViewProfile(profile: ImportProfile) {
    handleEditProfile(profile)
    setViewOnly(true)
  }

  // Duplicar: o formulário de um perfil NOVO, já preenchido com a cópia.
  // As regras que a tela não mostra (map, split...) vão junto ao salvar.
  function handleDuplicateProfile(profile: ImportProfile) {
    handleEditProfile(profile)
    setViewOnly(false)
    setEditingProfile(null)
    setProfileName(`${profile.name}-copia`)
  }

  function handleCancelEdit() {
    setImportSnapshot(null)
    setEditingProfile(null)
    setEditingOriginal(null)
    setViewOnly(false)
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
    if (!(await confirmDialog({
      title: 'Apagar perfil de import?',
      message: `O perfil "${name}" será apagado. Esta ação não pode ser desfeita.`,
      confirmLabel: 'Apagar perfil',
      tone: 'danger',
    }))) {
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

  // O valor de exemplo de uma coluna (a primeira linha do CSV ou a primeira
  // imagem lida): mostra ao lado da ligação o que vai entrar no campo.
  function sampleOf(column: string): string {
    const row = inspection?.sample[0]
    if (!row || !column) return ''
    return row[column.replace('{n}', '1')] ?? ''
  }

  // Uma ligação "campo ← coluna": de qual coluna vem, escolhida numa busca
  // entre as colunas/chaves lidas (não dá para inventar uma). No modo de
  // colunas numeradas, o nome é digitado (com {n}). Data e hora mapeada
  // abre embaixo o formato.
  function renderTarget(field: ProfileField, path: string, value: string, onChange: (column: string) => void, original: FieldRule | undefined, typed: boolean) {
    const sample = sampleOf(value)
    const id = `im-${path.replace(/[^a-z0-9]+/gi, '-')}`
    return (
      <li className={value ? 'im-row im-row--linked' : 'im-row'} key={path}>
        <div className="im-row__field">
          <label htmlFor={id}>{fieldLabel(field)}{field.required && <span className="im-req" title="Obrigatório para verificar"> *</span>}</label>
          {TYPE_HINTS[field.type] && <small>{TYPE_HINTS[field.type]}</small>}
        </div>
        <span aria-hidden="true" className="im-row__arrow">←</span>
        <div className="im-row__source">
          {typed ? (
            <input className="pe-mono" id={id} list="profile-columns" onChange={(event) => onChange(event.target.value.trim())} placeholder="Placa{n}" value={value} />
          ) : (
            <ColumnPicker columns={availableColumns} id={id} onChange={onChange} value={value} />
          )}
          {/* o valor de exemplo só quando há um exemplo analisado */}
          {value && inspection && (
            <small className={sample ? 'im-sample' : 'im-sample im-sample--none'}>
              {sample ? <>Exemplo: <code>{sample}</code></> : 'Vazio no exemplo'}
            </small>
          )}
          {advancedLabel(original) && <small className="im-kept">Mantido ao salvar: {advancedLabel(original)}</small>}
        </div>
        {field.type === 'datetime' && value && renderDate(path, value)}
      </li>
    )
  }

  // O formato de um campo de data e hora mapeado.
  function renderDate(path: string, column: string) {
    const date = dateSetting(path)
    const word = profileType === 'csv' ? 'coluna' : 'chave'
    const id = `im-date-${path.replace(/[^a-z0-9]+/gi, '-')}`
    return (
      <div className="im-date" key={`date-${path}`}>
        <div className="pe-field">
          <label htmlFor={`${id}-time`}>Hora em outra {word}</label>
          <select id={`${id}-time`} onChange={(event) => changeDate(path, { timeColumn: event.target.value })} value={date.timeColumn}>
            <option value="">A data já tem a hora</option>
            {availableColumns.filter((c) => c !== column).map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </div>
        <div className="pe-field">
          <label htmlFor={`${id}-ms`}>Milissegundo em outra {word}</label>
          <select id={`${id}-ms`} onChange={(event) => changeDate(path, { msColumn: event.target.value })} value={date.msColumn}>
            <option value="">Sem milissegundo separado</option>
            {availableColumns.filter((c) => c !== column && c !== date.timeColumn).map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </div>
        <div className="pe-field">
          <label htmlFor={`${id}-fmt`}>Formato</label>
          <select
            id={`${id}-fmt`}
            onChange={(event) => (event.target.value === 'custom'
              ? changeDate(path, { custom: true })
              : changeDate(path, { custom: false, format: event.target.value }))}
            value={date.custom ? 'custom' : date.format}
          >
            {DATE_PRESETS.map(([layout, label]) => <option key={layout} value={layout}>{label}</option>)}
            <option value="custom">Outro formato…</option>
          </select>
          {date.custom && (
            <input aria-label="Formato da data" className="pe-mono" onChange={(event) => changeDate(path, { format: event.target.value })} placeholder="02/01/2006 15:04:05" value={date.format} />
          )}
        </div>
        <div className="pe-field">
          <label htmlFor={`${id}-tz`}>Fuso horário</label>
          <input id={`${id}-tz`} onChange={(event) => changeDate(path, { timezone: event.target.value })} value={date.timezone} />
        </div>
        <small className="pe-muted im-date__help">
          {date.custom
            ? 'Escreva a data de referência 2006-01-02 15:04:05 no formato do arquivo (não uma data do seu arquivo), ex.: 02/01/2006 15:04:05.000.'
            : date.format ? `Layout: ${date.format}.` : 'Automático: dia antes do mês (DD/MM/AAAA) ou ano-mês-dia; hora com ou sem segundos.'}
          {sampleOf(column) && <> Exemplo lido: <code>{[sampleOf(column), date.timeColumn ? sampleOf(date.timeColumn) : '', date.msColumn ? sampleOf(date.msColumn) : ''].filter(Boolean).join(' ')}</code></>}
        </small>
      </div>
    )
  }

  // O que a tela edita num perfil de import (o arquivo de exemplo fica de fora:
  // trocar o exemplo não muda o perfil).
  const importState = JSON.stringify([profileType, profileName, profileDescription, annotationProfileName, delimiter, encoding,
    matchColumn, matchBy, fieldColumns, objectColumns, objectSettings, dateSettings])
  const importDirty = activeTab === 'profile_editor' && !viewOnly && importSnapshot !== null && importState !== importSnapshot
  // Abriu o editor (ou acabou de limpar): a foto é o que está na tela agora.
  // Durante a renderização (e não num efeito): os set* de abrir já valeram.
  if (activeTab === 'profile_editor' && importSnapshot === null) setImportSnapshot(importState)

  // quais perfis usam um cadastro (os campos de referência)
  const usedBy = (ref: 'equipment' | 'camera') => {
    const names = annotationProfiles.filter((p) => [...p.fields, ...p.objects.flatMap((o) => o.fields)].some((f) => f.type === 'ref' && f.ref === ref)).map((p) => p.name)
    return names.length ? names.join(', ') : 'nenhum perfil ainda'
  }

  // sub: um passo dentro do item de cima (o editor do perfil)
  const navItem = (tab: Tab, label: string, count?: number, sub = false, icon?: SettingsIconName) => (
    <button
      aria-current={activeTab === tab ? 'page' : undefined}
      className={`settings-nav-item${sub ? ' settings-nav-item--sub' : ''}${activeTab === tab ? ' settings-nav-item--active' : ''}`}
      onClick={() => setActiveTab(tab)}
      type="button"
    >
      {icon && <SettingsIcon name={icon} />}
      <span className="settings-nav-text">{label}</span>
      {count !== undefined && <span className="settings-nav-badge">{count}</span>}
    </button>
  )

  if (!loaded) return <SettingsSkeleton />

  return (
    <section className="package-page settings-page">
      <div className="page-heading">
        <div>
          <h1>Configurar</h1>
          <p>{admin ? 'Sua conta, o que se anota, como se importa e quem usa o SIDM.' : 'Sua conta, aparência e atalhos de teclado.'}</p>
        </div>
      </div>

      <div className="settings-layout">
        <label className="settings-nav-select">
          <span className="sr-only">Seção</span>
          <select onChange={(event) => setActiveTab(event.target.value as Tab)} value={activeTab}>
            {(Object.keys(TAB_LABELS) as Tab[]).filter((tab) => admin || USER_TABS.includes(tab)).map((tab) => <option key={tab} value={tab}>{TAB_LABELS[tab]}</option>)}
          </select>
        </label>
        <aside className="settings-sidebar">
          <nav aria-label="Seções do Configurar" className="settings-nav">
            <span className="settings-nav-section-title">Você</span>
            {navItem('account', 'Conta', undefined, false, 'account')}
            {navItem('shortcuts', 'Atalhos de teclado', undefined, false, 'shortcuts')}
            {admin && (
              <>
                <span className="settings-nav-section-title">Anotação</span>
                {navItem('annotation_profiles', 'Perfis de anotação', annotationSummary.count, false, 'profiles')}
                {navItem('annotation_editor', annotationSummary.editorLabel, undefined, true)}
                {navItem('equipments', 'Equipamentos', equipments.length, false, 'equipments')}
                {navItem('cameras', 'Câmeras', cameras.length, false, 'cameras')}

                <span className="settings-nav-section-title">Importação</span>
                {navItem('profiles_list', 'Perfis de importação', profiles.length, false, 'imports')}
                {navItem('profile_editor', viewOnly ? 'Ver perfil' : editingProfile ? 'Editar perfil' : 'Novo perfil', undefined, true)}

                <span className="settings-nav-section-title">Administração</span>
                {navItem('users', 'Usuários', undefined, false, 'users')}
                {navItem('system', 'Armazenamento', undefined, false, 'storage')}
              </>
            )}
          </nav>
        </aside>

        <main className="settings-content">
          {activeTab === 'account' && <AccountSection />}
          {activeTab === 'shortcuts' && <ShortcutsSection />}
          {activeTab === 'users' && admin && <UsersPage embedded />}
          {activeTab === 'system' && admin && <SystemSection />}
          {activeTab === 'equipments' && (
            <RegistrySection inactive={inactiveEquipments} items={equipments} kind="equipment" onChanged={loadCodes} usedBy={usedBy('equipment')} />
          )}
          {activeTab === 'cameras' && (
            <RegistrySection inactive={inactiveCameras} items={cameras} kind="camera" onChanged={loadCodes} usedBy={usedBy('camera')} />
          )}

          {/* Sempre montada: o rascunho não se perde ao trocar de tela. */}
          {admin && <AnnotationProfilesSection
            onSummary={setAnnotationSummary}
            onView={(view) => setActiveTab(view === 'list' ? 'annotation_profiles' : 'annotation_editor')}
            view={activeTab === 'annotation_profiles' ? 'list' : activeTab === 'annotation_editor' ? 'editor' : null}
          />}

          {activeTab === 'profiles_list' && (
            <div className="set-page">
              <SettingsHeader
                actions={<button className="button button--primary button--sm" onClick={() => { handleCancelEdit(); setActiveTab('profile_editor') }} type="button">Criar perfil</button>}
                description="Um perfil de importação lê os dados de um CSV ou do INI gravado nas imagens e preenche os rascunhos da anotação."
                title="Perfis de importação"
              />
              <ImportProfileList
                annotationProfiles={annotationProfiles}
                busy={saving !== ''}
                defaultAnnotation={DEFAULT_ANNOTATION_PROFILE}
                onDuplicate={handleDuplicateProfile}
                onOpen={(p) => (p.builtin ? handleViewProfile(p) : handleEditProfile(p))}
                onRemove={(name) => void handleDeleteProfile(name)}
                profiles={profiles}
              />
            </div>
          )}

          {activeTab === 'profile_editor' && (() => {
            const isIni = profileType === 'jpeg-ini'
            const word = isIni ? 'chave' : 'coluna'
            const hasColumns = inspection !== null || availableColumns.length > 0
            const allTargets = [...imageTargets.map((f) => fieldColumns[f.key]), ...objectTargets.flatMap((o) => o.fields.map((f) => objectColumns[o.key]?.[f.key]))]
            const linked = allTargets.filter(Boolean).length
            const close = async () => {
              if (importDirty && !(await confirmDialog({
                title: 'Descartar as alterações?',
                message: editingProfile ? `As mudanças no perfil "${editingProfile}" ainda não foram salvas.` : 'O perfil ainda não foi criado. O que foi preenchido se perde.',
                confirmLabel: 'Descartar alterações',
                tone: 'danger',
              }))) return
              handleCancelEdit()
              setActiveTab('profiles_list')
            }
            const chooseType = (type: 'csv' | 'jpeg-ini') => {
              setProfileType(type)
              setInspection(null)
              setAvailableColumns([])
              setFieldColumns({})
              setObjectColumns({})
              setObjectSettings({})
              setDateSettings({})
            }
            return (
            <form className="set-page pe-editor im-editor" onSubmit={(event) => void submitProfile(event)}>
              <SettingsHeader
                actions={viewOnly && editingOriginal ? <button className="button button--secondary button--sm" onClick={() => handleDuplicateProfile(editingOriginal)} type="button">Duplicar para editar</button> : undefined}
                description={viewOnly
                  ? 'Este é um perfil padrão do SIDM: só dá para ver. Para mudar, duplique.'
                  : isIni
                    ? 'Ligue as chaves do INI gravado dentro das imagens (Secao.Chave) aos campos da anotação.'
                    : 'Ligue as colunas do seu CSV aos campos da anotação.'}
                title={viewOnly ? `Perfil ${editingProfile}` : editingProfile ? `Editar ${editingProfile}` : 'Criar perfil de importação'}
              />

              {!editingProfile && (
                <fieldset className="pe-kind im-kind">
                  <legend>De onde vêm os dados</legend>
                  {([
                    ['csv', 'Planilha CSV', 'Um arquivo à parte, com uma linha por imagem (ou por veículo).'],
                    ['jpeg-ini', 'Dados gravados na imagem', 'O INI que os equipamentos gravam dentro de cada JPEG.'],
                  ] as const).map(([type, title, help]) => (
                    <label className={profileType === type ? 'pe-kind__opt pe-kind__opt--on' : 'pe-kind__opt'} key={type}>
                      <input checked={profileType === type} name="im-type" onChange={() => chooseType(type)} type="radio" />
                      <span><strong>{title}</strong><small>{help}</small></span>
                    </label>
                  ))}
                </fieldset>
              )}

              <fieldset className="profile-fieldset" disabled={viewOnly}>
                <SettingsGroup title="Sobre o perfil">
                  <SettingRow help="Letras sem acento, números e hífen; espaço vira hífen." htmlFor="im-name" label="Nome">
                    <input className="pe-mono pe-w-md" id="im-name" onChange={(event) => setProfileName(event.target.value)} placeholder={isIni ? 'ini-rodovia' : 'csv-rodovia'} value={profileName} />
                  </SettingRow>
                  <SettingRow help="Aparece ao escolher o perfil no envio de imagens." htmlFor="im-desc" label="Descrição">
                    <input className="pe-w-lg" id="im-desc" onChange={(event) => setProfileDescription(event.target.value)} placeholder={isIni ? 'INI dos equipamentos da rodovia' : 'Planilha da concessionária da rodovia'} value={profileDescription} />
                  </SettingRow>
                  <SettingRow
                    help={<>{annotationProfile?.description ? `${annotationProfile.description}. ` : ''}Os campos a ligar são os deste perfil; trocar apaga as ligações.</>}
                    htmlFor="im-target"
                    label="Preenche o perfil de anotação"
                  >
                    <select className="pe-w-md" id="im-target" onChange={(event) => changeAnnotationProfile(event.target.value)} value={annotationProfileName}>
                      {annotationProfiles.map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}
                      {!annotationProfile && <option value={annotationProfileName}>{annotationProfileName} (não existe mais)</option>}
                    </select>
                  </SettingRow>
                  {editingProfile && (
                    <SettingRow help="Não muda depois de criado: para ler de outra fonte, crie outro perfil." label="Lê de">
                      <span className="set-value">{isIni ? 'Dados gravados na imagem (INI)' : 'Planilha CSV'}</span>
                    </SettingRow>
                  )}
                  {!isIni && (
                    <>
                      <SettingRow help="O caractere entre as colunas do arquivo." htmlFor="im-delim" label="Separador">
                        <select className="pe-w-md" id="im-delim" onChange={(event) => setDelimiter(event.target.value)} value={delimiter}>
                          <option value="comma">Vírgula (,)</option>
                          <option value="semicolon">Ponto e vírgula (;)</option>
                          <option value="tab">Tabulação</option>
                          <option value="pipe">Barra vertical (|)</option>
                        </select>
                      </SettingRow>
                      <SettingRow help="Acentos trocados (Ã§ no lugar de ç)? Experimente Latin-1." htmlFor="im-enc" label="Codificação">
                        <select className="pe-w-md" id="im-enc" onChange={(event) => setEncoding(event.target.value)} value={encoding}>
                          <option value="utf-8">UTF-8</option>
                          <option value="latin1">Latin-1</option>
                        </select>
                      </SettingRow>
                    </>
                  )}
                </SettingsGroup>
              </fieldset>

              {!viewOnly && (
                <section aria-labelledby="im-sample-title" className="set-group">
                  <div className="set-group__head">
                    <h3 id="im-sample-title">{isIni ? 'Imagens de exemplo' : 'Arquivo de exemplo'}</h3>
                    <p>
                      {isIni
                        ? 'Até 20 imagens (podem ser criptografadas): o SIDM lê as chaves do INI delas para você escolher.'
                        : editingProfile ? 'Opcional: analise um CSV novo para ver outras colunas.' : 'O SIDM lê as colunas e a primeira linha para você escolher e conferir.'}
                    </p>
                  </div>
                  <div className="im-file">
                    <label className={(isIni ? profileImages.length : profileFile) ? 'im-file__drop im-file__drop--chosen' : 'im-file__drop'}>
                      <span className="im-file__name">
                        {isIni
                          ? profileImages.length ? (profileImages.length === 1 ? '1 imagem escolhida' : `${profileImages.length} imagens escolhidas`) : 'Escolher imagens'
                          : profileFile?.name ?? 'Escolher o CSV'}
                      </span>
                      <span className="im-file__hint">{isIni ? 'JPEG dos equipamentos' : '.csv'}</span>
                      {isIni ? (
                        // sem "accept": as criptografadas não têm tipo de imagem, e o backend as abre
                        <input className="sr-only" multiple onChange={(event) => { setProfileImages(imageFiles(event.target.files ?? []).slice(0, 20)); setInspection(null); setError('') }} type="file" />
                      ) : (
                        <input accept=".csv,text/csv" className="sr-only" onChange={(event) => { setProfileFile(event.target.files?.[0] ?? null); setInspection(null); setError('') }} type="file" />
                      )}
                    </label>
                    <button aria-busy={saving === 'inspect' || undefined} className="button button--secondary" disabled={saving === 'inspect'} onClick={() => void inspectProfileFile()} type="button">
                      {saving === 'inspect' ? 'Analisando…' : isIni ? 'Ler o INI' : 'Analisar CSV'}
                    </button>
                  </div>
                  {inspection && (
                    <p className="im-file__result" role="status">
                      {isIni
                        ? `${inspection.rows === 1 ? '1 imagem' : `${inspection.rows} imagens`} com INI, ${inspection.columns.length} chaves.`
                        : `${inspection.rows === 1 ? '1 linha' : `${inspection.rows} linhas`}, ${inspection.columns.length} colunas.`}
                    </p>
                  )}
                </section>
              )}

              {!hasColumns && (
                <div className="pe-empty">
                  <strong>Falta o exemplo</strong>
                  <span>Escolha {isIni ? 'algumas imagens' : 'o CSV'} e clique em {isIni ? 'Ler o INI' : 'Analisar CSV'} para ligar cada campo a uma {word}.</span>
                </div>
              )}

              {hasColumns && (
                <fieldset className="profile-fieldset" disabled={viewOnly}>
                  <datalist id="profile-columns">
                    {availableColumns.map((column) => <option key={column} value={column} />)}
                  </datalist>

                  {!isIni && (
                    <SettingsGroup description="Cada linha do CSV vale para a imagem com o mesmo nome (ou SHA1)." title="Como achar a imagem">
                      <SettingRow htmlFor="im-match" label="Coluna da imagem" help={sampleOf(matchColumn) ? <>Exemplo: <code>{sampleOf(matchColumn)}</code></> : undefined}>
                        <select className="pe-w-md" id="im-match" onChange={(event) => setMatchColumn(event.target.value)} value={matchColumn}>
                          {availableColumns.map((column) => <option key={column} value={column}>{column}</option>)}
                        </select>
                      </SettingRow>
                      <SettingRow htmlFor="im-by" label="Comparar com">
                        <select className="pe-w-md" id="im-by" onChange={(event) => setMatchBy(event.target.value as 'original_name' | 'sha1')} value={matchBy}>
                          <option value="original_name">Nome original do arquivo</option>
                          <option value="sha1">SHA1 da imagem</option>
                        </select>
                      </SettingRow>
                    </SettingsGroup>
                  )}

                  <section aria-labelledby="im-fields" className="set-group">
                    <div className="set-group__head">
                      <h3 id="im-fields">Campos da imagem</h3>
                      <p>Busque a {word} de cada campo de {annotationProfileName}. O que não existe no arquivo fica sem ligar.</p>
                    </div>
                    {imageTargets.length === 0
                      ? <div className="pe-empty"><span>Este perfil de anotação não tem campos na imagem.</span></div>
                      : (
                        <ul className="im-map">
                          {imageTargets.map((field) => renderTarget(field, field.key, fieldColumns[field.key] ?? '',
                            (column) => setFieldColumns((current) => ({ ...current, [field.key]: column })),
                            originalFields?.[field.key], false))}
                        </ul>
                      )}
                  </section>

                  {objectTargets.map((target) => {
                    const setting = objectSetting(target.key)
                    const columns = objectColumns[target.key] ?? {}
                    const setSetting = (patch: Partial<ObjectSetting>) =>
                      setObjectSettings((current) => ({ ...current, [target.key]: { ...setting, ...patch } }))
                    return (
                      <section aria-labelledby={`im-obj-${target.key}`} className="set-group" key={target.key}>
                        <div className="set-group__head">
                          <h3 id={`im-obj-${target.key}`}>{fieldLabel(target)}</h3>
                          <p>Os campos de cada {fieldLabel(target).toLowerCase()} da imagem.</p>
                        </div>
                        <div className="im-object-mode">
                          <div className="pe-field">
                            <label htmlFor={`im-mode-${target.key}`}>Como aparecem no arquivo</label>
                            {/* no CSV, "single" e "row" são o mesmo: um objeto por linha */}
                            <select
                              id={`im-mode-${target.key}`}
                              onChange={(event) => setSetting({ mode: event.target.value as ObjectSetting['mode'] })}
                              value={setting.mode === 'columns' ? 'columns' : profileType === 'csv' ? 'row' : 'single'}
                            >
                              {profileType === 'csv'
                                ? <option value="row">Um por linha (as linhas da mesma imagem se juntam)</option>
                                : <option value="single">Um por imagem</option>}
                              <option value="columns">Colunas numeradas (Placa1, Placa2…)</option>
                            </select>
                          </div>
                          {setting.mode === 'columns' && (
                            <div className="pe-field">
                              <label htmlFor={`im-max-${target.key}`}>Quantos, no máximo</label>
                              <input id={`im-max-${target.key}`} max={20} min={1} onChange={(event) => setSetting({ max: Number.parseInt(event.target.value, 10) || 1 })} type="number" value={setting.max ?? 3} />
                            </div>
                          )}
                        </div>
                        {setting.mode === 'columns' && (
                          <p className="pe-muted im-object-help">Escreva o nome da {word} com {'{n}'} no lugar do número, ex.: Placa{'{n}'}.</p>
                        )}
                        <ul className="im-map">
                          {target.fields.map((field) => renderTarget(field, `${target.key}.${field.key}`, columns[field.key] ?? '',
                            (column) => setObjectColumns((current) => ({ ...current, [target.key]: { ...(current[target.key] ?? {}), [field.key]: column } })),
                            originalObjects?.[target.key]?.fields[field.key], setting.mode === 'columns'))}
                        </ul>
                      </section>
                    )
                  })}
                </fieldset>
              )}

              <div className={importDirty ? 'set-savebar set-savebar--dirty' : 'set-savebar'} role="status">
                <span>
                  {saving === 'profile' ? 'Salvando…'
                    : viewOnly ? 'Perfil padrão.'
                      : importDirty && hasColumns ? `Alterações não salvas: ${linked} de ${allTargets.length} campos ligados.`
                      : hasColumns ? `${linked} de ${allTargets.length} campos ligados.` : 'Falta analisar um exemplo.'}
                </span>
                <div>
                  <button className="button button--secondary button--sm" onClick={() => void close()} type="button">{importDirty ? 'Descartar' : 'Voltar à lista'}</button>
                  {!viewOnly && (
                    <button aria-busy={saving === 'profile' || undefined} className="button button--primary button--sm" disabled={saving === 'profile'} type="submit">
                      {saving === 'profile' ? 'Salvando…' : editingProfile ? 'Salvar alterações' : 'Criar perfil'}
                    </button>
                  )}
                </div>
              </div>
            </form>
            )
          })()}
        </main>
      </div>
    </section>
  )
}
