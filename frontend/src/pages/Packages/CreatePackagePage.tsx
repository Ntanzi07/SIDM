import { useEffect, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { createPackage, imageFiles, importPackageCsv, listAnnotationProfiles, listImportProfiles, listIniProfiles, uploadImages, type ImportProfile, type ImportReport, type UploadProgress } from '../../services/api'
import { useFolderReading } from '../../hooks/useFolderReading'
import type { AnnotationProfile } from '../../types/annotation'
import { ImportWarnings } from './ImportWarnings'
import { UploadStatus } from './UploadStatus'
import { defaultIniProfile, forPackage, optionLabel } from '../../utils/importProfiles'
import './Packages.css'

export function CreatePackagePage() {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [folder, setFolder] = useState<FileList | null>(null)
  const [annotationProfiles, setAnnotationProfiles] = useState<AnnotationProfile[]>([])
  const [annotationProfile, setAnnotationProfile] = useState('veiculos')
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [iniProfiles, setIniProfiles] = useState<ImportProfile[]>([])
  const [iniProfile, setIniProfile] = useState('')
  const [profileName, setProfileName] = useState('')
  const [csvFile, setCsvFile] = useState<File | null>(null)
  const [overwrite, setOverwrite] = useState(true)
  const [error, setError] = useState('')
  const [profileError, setProfileError] = useState('')
  const [saving, setSaving] = useState<'creating' | 'uploading' | 'testing' | 'applying' | ''>('')
  const [upload, setUpload] = useState<UploadProgress | null>(null)
  const [createdPackageName, setCreatedPackageName] = useState('')
  const [importReport, setImportReport] = useState<ImportReport | null>(null)
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
    setError('')
  }

  function handleCsvChange(event: ChangeEvent<HTMLInputElement>) {
    setCsvFile(event.target.files?.[0] ?? null)
    setError('')
    setImportReport(null)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
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
      const report = await importPackageCsv(createdName, profileName, csvFile, true, overwrite)
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
    try {
      await importPackageCsv(createdPackageName, profileName, csvFile, false, overwrite)
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
            <label htmlFor="package-folder">Pasta das imagens</label>
            <label className={readingFolder ? 'folder-picker folder-picker--reading' : 'folder-picker'} htmlFor="package-folder">
              <span className="folder-picker__icon">↥</span>
              <span>
                {readingFolder ? (
                  <>
                    <strong>Lendo a pasta...</strong>
                    <small>Pastas grandes levam alguns segundos. Se o navegador perguntar, confirme o envio dos arquivos.</small>
                  </>
                ) : (
                  <>
                    <strong>{folder?.[0]?.webkitRelativePath.split('/')[0] ?? 'Selecione uma pasta'}</strong>
                    <small>{folder ? `${folder.length} arquivo(s) selecionado(s)` : 'Escolha a pasta que contém as imagens'}</small>
                  </>
                )}
              </span>
              <input
                id="package-folder"
                onChange={handleFolderChange}
                onClick={startReadingFolder}
                ref={folderInputRef}
                type="file"
                {...{ webkitdirectory: 'true' }}
              />
            </label>
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
              disabled={saving !== ''}
              onChange={(event) => {
                setProfileName(event.target.value)
                setError('')
                setImportReport(null)
              }}
              value={profileName}
            >
              <option value="">Não importar CSV</option>
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
        {error && <p className="form-error">{error}</p>}
        {importReport && (
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
              {saving === 'applying' ? 'Aplicando...' : 'Aplicar importação e abrir pacote'}
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
                : saving === 'testing' ? 'Ensaiando CSV...'
                  : createdPackageName ? 'Tentar de novo' : 'Criar pacote'}
          </button>
        </div>
      </form>
    </section>
  )
}
