import { useEffect, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { createPackage, imageFiles, listAnnotationProfiles, listImportProfiles, listIniProfiles, runImport, sendZip, uploadImages, type ImportProfile, type ImportReport, type ImportStep, type UploadProgress, type ZipUpload } from '../../services/api'
import { ImportProgress } from './ImportProgress'
import { useFolderReading } from '../../hooks/useFolderReading'
import type { AnnotationProfile } from '../../types/annotation'
import { ImportWarnings } from './ImportWarnings'
import { UploadStatus } from './UploadStatus'
import { FolderOrZip } from './FolderOrZip'
import { defaultIniProfile, forPackage, optionLabel } from '../../utils/importProfiles'
import { useToastState } from '../../hooks/useToast'
import './Packages.css'

export function CreatePackagePage() {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [folder, setFolder] = useState<FileList | null>(null)
  // ou um .zip, para pastas grandes (o navegador trava listando a pasta)
  const [zipFile, setZipFile] = useState<File | null>(null)
  const [zipSent, setZipSent] = useState<ZipUpload | null>(null)
  const [annotationProfiles, setAnnotationProfiles] = useState<AnnotationProfile[]>([])
  const [annotationProfile, setAnnotationProfile] = useState('veiculos')
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [iniProfiles, setIniProfiles] = useState<ImportProfile[]>([])
  const [iniProfile, setIniProfile] = useState('')
  const [profileName, setProfileName] = useState('')
  const [csvFile, setCsvFile] = useState<File | null>(null)
  const [overwrite, setOverwrite] = useState(true)
  const [, setError] = useToastState('error')
  const [profileError, setProfileError] = useState('')
  const [saving, setSaving] = useState<'creating' | 'uploading' | 'testing' | 'applying' | ''>('')
  const [upload, setUpload] = useState<UploadProgress | null>(null)
  const [createdPackageName, setCreatedPackageName] = useState('')
  const [importReport, setImportReport] = useState<ImportReport | null>(null)
  // o andamento do ensaio/aplicar do CSV (o servidor manda as etapas)
  const [importStep, setImportStep] = useState<ImportStep | null>(null)
  const [importStartedAt, setImportStartedAt] = useState(0)
  const { reading: readingFolder, start: startReadingFolder, done: doneReadingFolder, inputRef: folderInputRef } = useFolderReading()

  useEffect(() => {
    listAnnotationProfiles().then(setAnnotationProfiles).catch(() => { /* sem a lista: fica o padrão */ })
    listIniProfiles().then((list) => {
      setIniProfiles(list)
      setIniProfile((current) => current || defaultIniProfile(list))
    }).catch(() => { /* sem perfis de INI: o seletor não aparece */ })
    listImportProfiles()
      .then(setProfiles)
      .catch((requestError: unknown) => setProfileError(
        requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis CSV.',
      ))
  }, [])

  function handleFolderChange(event: ChangeEvent<HTMLInputElement>) {
    doneReadingFolder()
    setFolder(event.target.files)
    setZipFile(null)
    setError('')
  }

  function handleZipChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0] ?? null
    event.target.value = ''
    if (file && !file.name.toLowerCase().endsWith('.zip')) {
      setError('Escolha um arquivo .zip.')
      return
    }
    setZipFile(file)
    if (file) {
      setFolder(null)
      setProfileName('') // o CSV vai depois, no pacote, quando as imagens estiverem gravadas
    }
    setError('')
  }

  // O zip: cria o pacote, manda em partes e abre o pacote (a gravação das
  // imagens segue no servidor, e o progresso aparece lá).
  async function submitZip(file: File) {
    setSaving('creating')
    setError('')
    try {
      const createdName = createdPackageName || await createPackage(name.trim(), annotationProfile)
      setCreatedPackageName(createdName)
      setSaving('uploading')
      await sendZip(createdName, file, iniProfile || undefined, setZipSent)
      navigate(`/pacotes/${encodeURIComponent(createdName)}`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error
        ? `${requestError.message} Clique de novo para continuar o envio de onde parou.`
        : 'Não foi possível enviar o zip.')
    } finally {
      setSaving('')
    }
  }

  function handleCsvChange(event: ChangeEvent<HTMLInputElement>) {
    setCsvFile(event.target.files?.[0] ?? null)
    setError('')
    setImportReport(null)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (name.trim() && zipFile) {
      void submitZip(zipFile)
      return
    }
    if (!name.trim() || !folder || folder.length === 0) {
      setError('Informe um nome e selecione a pasta de imagens para continuar.')
      return
    }
    if (Boolean(profileName) !== Boolean(csvFile)) {
      setError('Para importar o CSV, selecione um perfil e um arquivo CSV.')
      return
    }

    const files = imageFiles(folder)
    if (files.length === 0) {
      setError('A pasta selecionada não tem imagens.')
      return
    }

    setSaving('creating')
    setError('')
    setImportReport(null)
    try {
      // Se o pacote já foi criado numa tentativa anterior, não cria de novo:
      // o upload pula as imagens que já subiram (duplicadas).
      const createdName = createdPackageName || await createPackage(name.trim(), annotationProfile)
      setCreatedPackageName(createdName)

      setSaving('uploading')
      const result = await uploadImages(createdName, files, setUpload, iniProfile || undefined)
      if (result.errors.length > 0) {
        setError(`${result.errors.length} imagem(ns) não subiram. Corrija e clique de novo para reenviar só o que falta.`)
        return
      }
      if (!csvFile || !profileName) {
        navigate(`/pacotes/${encodeURIComponent(createdName)}`)
        return
      }

      setSaving('testing')
      setImportStep(null)
      setImportStartedAt(Date.now())
      const report = await runImport(createdName, profileName, csvFile, true, overwrite, setImportStep)
      setImportReport(report)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível criar o pacote.')
    } finally {
      setSaving('')
    }
  }

  async function handleApplyImport() {
    if (!createdPackageName || !profileName || !csvFile) return
    setSaving('applying')
    setError('')
    setImportStep(null)
    setImportStartedAt(Date.now())
    try {
      await runImport(createdPackageName, profileName, csvFile, false, overwrite, setImportStep)
      navigate(`/pacotes/${encodeURIComponent(createdPackageName)}`)
    } catch (requestError: unknown) {
      setError(requestError instanceof Error ? requestError.message : 'Não foi possível aplicar a importação.')
    } finally {
      setSaving('')
    }
  }

  return (
    <section className="package-page create-package-page">
      <button className="back-button" onClick={() => navigate('/pacotes')} type="button">
        ← Voltar para pacotes
      </button>
      <div className="page-heading">
        <div>
          <h1>Criar pacote</h1>
          <p>Organize as imagens que serão usadas em um novo pacote de anotação.</p>
        </div>
      </div>

      <form className="create-card" onSubmit={handleSubmit}>
        <div className="form-field">
          <label htmlFor="package-name">Nome do pacote</label>
          <input
            disabled={Boolean(createdPackageName)}
            id="package-name"
            onChange={(event) => setName(event.target.value)}
            placeholder="Ex.: Placas BR - Rodovia Anhanguera"
            value={name}
          />
        </div>
        <div className="form-row">
          <div className="form-field">
            <label htmlFor="annotation-profile">O que anotar (perfil de anotação)</label>
            <select
              disabled={Boolean(createdPackageName)}
              id="annotation-profile"
              onChange={(event) => setAnnotationProfile(event.target.value)}
              value={annotationProfile}
            >
              {annotationProfiles.length === 0 && <option value="veiculos">veiculos</option>}
              {annotationProfiles.map((profile) => (
                <option key={profile.name} value={profile.name}>{profile.name}</option>
              ))}
            </select>
            <small className="form-hint">
              {annotationProfiles.find((profile) => profile.name === annotationProfile)?.description
                ? `${annotationProfiles.find((profile) => profile.name === annotationProfile)?.description}. `
                : ''}
              Os campos da tela de anotação vêm do perfil. Dá para trocar depois, enquanto nenhuma imagem foi verificada.
            </small>
          </div>
          <div className="form-field">
            <label>Imagens (pasta ou .zip)</label>
            <FolderOrZip
              className={readingFolder ? 'folder-picker folder-picker--reading' : 'folder-picker'}
              disabled={saving !== ''}
              folderRef={folderInputRef}
              onFolderChange={handleFolderChange}
              onFolderClick={startReadingFolder}
              onZipChange={handleZipChange}
            >
              <span className="folder-picker__icon">↥</span>
              <span>
                {readingFolder ? (
                  <>
                    <strong>Lendo a pasta...</strong>
                    <small>Pastas grandes levam alguns segundos. Se o navegador perguntar, confirme o envio dos arquivos.</small>
                  </>
                ) : zipFile ? (
                  <>
                    <strong>{zipFile.name}</strong>
                    <small>Zip de {(zipFile.size / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 0 })} MB · vai em partes</small>
                  </>
                ) : (
                  <>
                    <strong>{folder?.[0]?.webkitRelativePath.split('/')[0] ?? 'Selecione uma pasta ou um .zip'}</strong>
                    <small>{folder ? `${folder.length} arquivo(s) selecionado(s)` : 'Para pastas muito grandes, use um .zip: vai em partes e continua de onde parou'}</small>
                  </>
                )}
              </span>
            </FolderOrZip>
          </div>
        </div>
        <div className="form-row">
          {iniProfiles.length > 0 && (
            <div className="form-field import-field">
              <label htmlFor="ini-profile">Ler o INI embutido nas imagens (opcional)</label>
              <select
                id="ini-profile"
                disabled={saving !== '' || Boolean(createdPackageName)}
                onChange={(event) => setIniProfile(event.target.value)}
                value={iniProfile}
              >
                <option value="">Não preencher pelo INI</option>
                {forPackage(iniProfiles, annotationProfile).map((profile) => (
                  <option key={profile.name} value={profile.name}>{optionLabel(profile, annotationProfile)}</option>
                ))}
              </select>
              <small className="form-hint">
                O INI de cada imagem é guardado de qualquer jeito. Com um perfil, ele já preenche os rascunhos no upload
                (dá para aplicar depois, no detalhe do pacote).
              </small>
            </div>
          )}
          <div className="form-field import-field">
            <label htmlFor="import-profile">Importar anotações do CSV (opcional)</label>
            <select
              id="import-profile"
              disabled={saving !== '' || Boolean(zipFile)}
              onChange={(event) => {
                setProfileName(event.target.value)
                setError('')
                setImportReport(null)
              }}
              value={profileName}
            >
              <option value="">{zipFile ? 'Com zip: importe o CSV no pacote, depois de gravar' : 'Não importar CSV'}</option>
              {forPackage(profiles, annotationProfile).map((profile) => (
                <option key={profile.name} value={profile.name}>{optionLabel(profile, annotationProfile)}</option>
              ))}
            </select>
            {profileError && <small className="form-hint form-hint--error">{profileError}</small>}
          </div>
        </div>
        {profileName && (
          <>
            <div className="form-field import-field">
              <label htmlFor="import-csv">Arquivo CSV</label>
              <input id="import-csv" accept=".csv,text/csv" disabled={saving !== ''} onChange={handleCsvChange} type="file" />
              {csvFile && <small className="form-hint">{csvFile.name}</small>}
            </div>
            <label className="import-option">
              <input checked={overwrite} disabled={saving !== ''} onChange={(event) => setOverwrite(event.target.checked)} type="checkbox" />
              <span>Sobrescrever rascunhos existentes</span>
            </label>
          </>
        )}
        {upload && <UploadStatus progress={upload} />}
        {zipSent && zipSent.status === 'receiving' && (
          <div className="upload-status">
            <div className="upload-status__bar"><span style={{ width: `${Math.round((zipSent.received / zipSent.size) * 100)}%` }} /></div>
            <p className="form-hint">
              Enviando o zip: {Math.round((zipSent.received / zipSent.size) * 100)}% ({(zipSent.received / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 0 })} de {(zipSent.size / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 0 })} MB).
              Não feche esta aba; no fim, o pacote abre e as imagens são gravadas no servidor.
            </p>
          </div>
        )}
        {(saving === 'testing' || saving === 'applying') && (
          <ImportProgress dryRun={saving === 'testing'} source="csv" startedAt={importStartedAt} step={importStep} />
        )}
        {importReport && saving !== 'applying' && (
          <div className="import-report">
            <h2>Ensaio da importação</h2>
            <div className="import-report__stats">
              <span><strong>{importReport.rows}</strong> linhas lidas</span>
              <span><strong>{importReport.matched}</strong> imagens encontradas</span>
              <span><strong>{importReport.applied}</strong> alterações previstas</span>
              <span><strong>{importReport.not_found_count}</strong> não encontradas</span>
            </div>
            <ImportWarnings report={importReport} />
            <button className="button button--primary" disabled={saving !== ''} onClick={() => void handleApplyImport()} type="button">
              Aplicar importação e abrir pacote
            </button>
          </div>
        )}
        <div className="create-card__footer">
          <button className="button button--secondary" onClick={() => navigate('/pacotes')} type="button">
            Cancelar
          </button>
          <button className="button button--primary" disabled={saving !== '' || Boolean(importReport) || readingFolder} type="submit">
            {saving === 'creating' ? 'Criando...'
              : saving === 'uploading' ? 'Enviando imagens...'
                : saving === 'testing' ? 'Ensaiando CSV…'
                  : createdPackageName ? 'Tentar de novo' : 'Criar pacote'}
          </button>
        </div>
      </form>
    </section>
  )
}
