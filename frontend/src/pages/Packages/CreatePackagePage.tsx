import { useEffect, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { createPackage, importPackageCsv, listImportProfiles, type ImportProfile, type ImportReport } from '../../services/api'
import './Packages.css'

export function CreatePackagePage() {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [folder, setFolder] = useState<FileList | null>(null)
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [profileName, setProfileName] = useState('')
  const [csvFile, setCsvFile] = useState<File | null>(null)
  const [overwrite, setOverwrite] = useState(true)
  const [error, setError] = useState('')
  const [profileError, setProfileError] = useState('')
  const [saving, setSaving] = useState<'creating' | 'testing' | 'applying' | ''>('')
  const [createdPackageName, setCreatedPackageName] = useState('')
  const [importReport, setImportReport] = useState<ImportReport | null>(null)

  useEffect(() => {
    listImportProfiles()
      .then(setProfiles)
      .catch((requestError: unknown) => setProfileError(
        requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis CSV.',
      ))
  }, [])

  function handleFolderChange(event: ChangeEvent<HTMLInputElement>) {
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

    setSaving('creating')
    setError('')
    setImportReport(null)
    try {
      const createdName = await createPackage(name.trim(), folder)
      setCreatedPackageName(createdName)
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
      <div className="page-heading">
        <div>
          <button className="back-button" onClick={() => navigate('/pacotes')} type="button">
            ← Voltar para pacotes
          </button>
          <h1>Criar pacote</h1>
          <p>Organize as imagens que serão usadas em um novo pacote de anotação.</p>
        </div>
      </div>

      <form className="create-card" onSubmit={handleSubmit}>
        <div className="form-field">
          <label htmlFor="package-name">Nome do pacote</label>
          <input
            id="package-name"
            onChange={(event) => setName(event.target.value)}
            placeholder="Ex.: Placas BR - Rodovia Anhanguera"
            value={name}
          />
        </div>
        <div className="form-field">
          <label htmlFor="package-folder">Pasta das imagens</label>
          <label className="folder-picker" htmlFor="package-folder">
            <span className="folder-picker__icon">↥</span>
            <span>
              <strong>{folder?.[0]?.webkitRelativePath.split('/')[0] ?? 'Selecione uma pasta'}</strong>
              <small>{folder ? `${folder.length} arquivo(s) selecionado(s)` : 'Escolha a pasta que contém as imagens'}</small>
            </span>
            <input
              id="package-folder"
              onChange={handleFolderChange}
              type="file"
              {...{ webkitdirectory: 'true' }}
            />
          </label>
        </div>
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
            {profiles.map((profile) => (
              <option key={profile.name} value={profile.name}>{profile.name}</option>
            ))}
          </select>
          {profileError && <small className="form-hint form-hint--error">{profileError}</small>}
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
            {(importReport.error_count > 0 || importReport.not_found_count > 0 || importReport.invalid_names.length > 0) && (
              <p className="import-report__warning">
                Revise os avisos antes de aplicar. O ensaio não alterou nenhuma imagem.
              </p>
            )}
            {Object.entries(importReport.unmapped).map(([field, values]) => (
              <p className="import-report__detail" key={field}>
                <strong>{field}:</strong> valores sem tradução ({Object.keys(values).length})
              </p>
            ))}
            {(importReport.new_equipments.length > 0 || importReport.new_cameras.length > 0) && (
              <p className="import-report__detail">
                <strong>Cadastros novos:</strong>{' '}
                {[...importReport.new_equipments, ...importReport.new_cameras].join(', ')}
              </p>
            )}
            {importReport.errors.slice(0, 5).map((item) => (
              <p className="import-report__detail import-report__detail--warning" key={`${item.line}-${item.field}-${item.key}`}>
                Linha {item.line}, {item.field}: {item.error}
              </p>
            ))}
            {importReport.invalid_names.slice(0, 5).map((item) => (
              <p className="import-report__detail import-report__detail--warning" key={`${item.line}-${item.field}-${item.value}`}>
                Linha {item.line}, {item.field}: {item.error}
              </p>
            ))}
            <button className="button button--primary" disabled={saving !== ''} onClick={() => void handleApplyImport()} type="button">
              {saving === 'applying' ? 'Aplicando...' : 'Aplicar importação e abrir pacote'}
            </button>
          </div>
        )}
        <div className="create-card__footer">
          <button className="button button--secondary" onClick={() => navigate('/pacotes')} type="button">
            Cancelar
          </button>
          <button className="button button--primary" disabled={saving !== '' || Boolean(importReport)} type="submit">
            {saving === 'creating' ? 'Criando...' : saving === 'testing' ? 'Ensaiando CSV...' : 'Criar pacote'}
          </button>
        </div>
      </form>
    </section>
  )
}
