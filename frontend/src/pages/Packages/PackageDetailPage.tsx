import { useNavigate, useParams } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { applyIniProfile, deleteImage, deletePackage, getPackage, imageFiles, importPackageCsv, listImportProfiles, listIniProfiles, listPackageImages, uploadImages, type ImageSummary, type ImportProfile, type ImportReport, type PackageSummary, type UploadProgress } from '../../services/api'
import type { ChangeEvent, FormEvent } from 'react'
import { UploadStatus } from './UploadStatus'
import { BoxesSection } from './BoxesSection'
import { ImportWarnings } from './ImportWarnings'
import './Packages.css'

const PAGE_SIZE = 100

export function PackageDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams()
  const [selectedPackage, setSelectedPackage] = useState<PackageSummary | undefined>()
  const [images, setImages] = useState<ImageSummary[]>([])
  const [imageTotal, setImageTotal] = useState(0)
  const [upload, setUpload] = useState<UploadProgress | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [iniProfiles, setIniProfiles] = useState<ImportProfile[]>([])
  // De onde vêm os metadados: um CSV enviado agora, ou o INI que já está
  // guardado em cada imagem (sem arquivo).
  const [importSource, setImportSource] = useState<'csv' | 'ini'>('csv')
  const [uploadIniProfile, setUploadIniProfile] = useState('')
  const [importProfile, setImportProfile] = useState('')
  const [importFile, setImportFile] = useState<File | null>(null)
  const [overwriteImport, setOverwriteImport] = useState(true)
  const [importReport, setImportReport] = useState<ImportReport | null>(null)
  const [importError, setImportError] = useState('')
  const [importSaving, setImportSaving] = useState<'testing' | 'applying' | ''>('')
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteConfirmationInput, setDeleteConfirmationInput] = useState('')

  const packageName = decodeURIComponent(id ?? '')

  // Recarrega o resumo e a PRIMEIRA página de imagens (as outras vêm no
  // "Carregar mais": um pacote pode ter dezenas de milhares).
  async function refresh() {
    const [summary, page] = await Promise.all([getPackage(packageName), listPackageImages(packageName, PAGE_SIZE, 0)])
    setSelectedPackage(summary)
    setImages(page.items)
    setImageTotal(page.total)
  }

  useEffect(() => {
    let active = true
    Promise.all([getPackage(packageName), listPackageImages(packageName, PAGE_SIZE, 0)])
      .then(([summary, page]) => {
        if (!active) return
        setSelectedPackage(summary)
        setImages(page.items)
        setImageTotal(page.total)
      })
      .catch((requestError: unknown) => { if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar o pacote.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [packageName])

  function handleLoadMore() {
    setSaving(true)
    listPackageImages(packageName, PAGE_SIZE, images.length)
      .then((page) => {
        setImages((current) => [...current, ...page.items])
        setImageTotal(page.total)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar mais imagens.'))
      .finally(() => setSaving(false))
  }

  useEffect(() => {
    listIniProfiles().then(setIniProfiles).catch(() => { /* sem perfis de INI: a opção some */ })
    listImportProfiles()
      .then(setProfiles)
      .catch((requestError: unknown) => setImportError(
        requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis CSV.',
      ))
  }, [])

  function handleAddImages(event: ChangeEvent<HTMLInputElement>) {
    if (!event.target.files || !selectedPackage) return
    const files = imageFiles(event.target.files)
    event.target.value = ''
    if (files.length === 0) {
      setError('Nenhuma imagem na seleção.')
      return
    }
    setSaving(true)
    setError('')
    uploadImages(selectedPackage.name, files, setUpload, uploadIniProfile || undefined)
      .then((result) => {
        if (result.errors.length > 0) setError(`${result.errors.length} imagem(ns) não subiram. Selecione de novo para reenviar: as que já subiram são puladas.`)
        return refresh()
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível adicionar as imagens.'))
      .finally(() => setSaving(false))
  }

  function handleRemoveImage(image: ImageSummary) {
    if (!window.confirm(`Remover permanentemente a imagem "${image.original_name}"?`)) return
    setSaving(true)
    setError('')
    deleteImage(image.sha1)
      .then(() => {
        setImages((current) => current.filter((item) => item.sha1 !== image.sha1))
        setImageTotal((total) => total - 1)
        return getPackage(packageName).then(setSelectedPackage)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível remover a imagem.'))
      .finally(() => setSaving(false))
  }

  function handleOpenDeleteModal() {
    setDeleteConfirmationInput('')
    setError('')
    setShowDeleteModal(true)
  }

  function handleCloseDeleteModal() {
    if (saving) return
    setShowDeleteModal(false)
    setDeleteConfirmationInput('')
  }

  function handleConfirmDelete(event: FormEvent) {
    event.preventDefault()
    if (!selectedPackage) return
    if (deleteConfirmationInput.trim() !== selectedPackage.name) {
      setError(`O nome digitado não corresponde a "${selectedPackage.name}".`)
      return
    }

    setSaving(true)
    setError('')
    deletePackage(selectedPackage.name)
      .then(() => navigate('/pacotes'))
      .catch((requestError: unknown) => {
        setError(requestError instanceof Error ? requestError.message : 'Não foi possível excluir o pacote.')
        setSaving(false)
      })
  }

  async function handleImport(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedPackage || !importProfile || (importSource === 'csv' && !importFile)) {
      setImportError(importSource === 'csv' ? 'Selecione um perfil e um arquivo CSV.' : 'Selecione um perfil de INI.')
      return
    }

    setImportSaving('testing')
    setImportError('')
    try {
      const report = importSource === 'ini'
        ? await applyIniProfile(selectedPackage.name, importProfile, true, overwriteImport)
        : await importPackageCsv(selectedPackage.name, importProfile, importFile as File, true, overwriteImport)
      setImportReport(report)
    } catch (requestError: unknown) {
      setImportError(requestError instanceof Error ? requestError.message : 'Não foi possível ensaiar a importação.')
    } finally {
      setImportSaving('')
    }
  }

  async function handleApplyImport() {
    if (!selectedPackage || !importProfile || (importSource === 'csv' && !importFile)) return
    setImportSaving('applying')
    setImportError('')
    try {
      if (importSource === 'ini') await applyIniProfile(selectedPackage.name, importProfile, false, overwriteImport)
      else await importPackageCsv(selectedPackage.name, importProfile, importFile as File, false, overwriteImport)
      await refresh()
      setImportReport(null)
    } catch (requestError: unknown) {
      setImportError(requestError instanceof Error ? requestError.message : 'Não foi possível aplicar a importação.')
    } finally {
      setImportSaving('')
    }
  }

  if (loading) {
    return <section className="package-page empty-package"><p>Carregando pacote...</p></section>
  }

  if (error || !selectedPackage) {
    return (
      <section className="package-page empty-package">
        <h1>{error ? 'Não foi possível carregar o pacote' : 'Pacote não encontrado'}</h1>
        {error && <p>{error}</p>}
        <button className="button button--primary" onClick={() => navigate('/pacotes')} type="button">
          Voltar para pacotes
        </button>
      </section>
    )
  }

  return (
    <section className="package-page">
      <button className="back-button" onClick={() => navigate('/pacotes')} type="button">
        ← Voltar para pacotes
      </button>
      <div className="detail-heading">
        <div>
          <h1>{selectedPackage.name}</h1>
          <p>criado por {selectedPackage.created_by}</p>
        </div>
        <div className="detail-heading__actions">
          <button className="button button--danger" disabled={saving} onClick={handleOpenDeleteModal} type="button">Excluir pacote</button>
          <button className="button button--primary" onClick={() => navigate(`/pacotes/${encodeURIComponent(selectedPackage.name)}/anotacao`)} type="button">Abrir anotação</button>
        </div>
      </div>
      <div className="detail-stats">
        <div><strong>{selectedPackage.total.toLocaleString('pt-BR')}</strong><span>Imagens</span></div>
        <div><strong>{selectedPackage.verified.toLocaleString('pt-BR')}</strong><span>Verificadas</span></div>
        <div><strong>{selectedPackage.pending.toLocaleString('pt-BR')}</strong><span>Pendentes</span></div>
      </div>
      <div className="detail-placeholder">
        <form className="existing-import" onSubmit={(event) => void handleImport(event)}>
          <div className="detail-images__heading">
            <div>
              <h2>Importar anotações</h2>
              <p>
                {importSource === 'csv'
                  ? 'Preencha os rascunhos deste pacote com um CSV, usando um perfil existente.'
                  : 'Preencha os rascunhos das imagens pendentes com o INI que veio dentro de cada JPEG.'}
              </p>
            </div>
            {iniProfiles.length > 0 && (
              <div className="import-source" role="radiogroup" aria-label="Origem dos metadados">
                {(['csv', 'ini'] as const).map((source) => (
                  <button
                    aria-checked={importSource === source}
                    className={importSource === source ? 'import-source__option import-source__option--active' : 'import-source__option'}
                    disabled={importSaving !== ''}
                    key={source}
                    onClick={() => {
                      setImportSource(source)
                      setImportProfile('')
                      setImportReport(null)
                      setImportError('')
                    }}
                    role="radio"
                    type="button"
                  >
                    {source === 'csv' ? 'CSV' : 'INI embutido'}
                  </button>
                ))}
              </div>
            )}
          </div>
          <div className="existing-import__controls">
            <select
              aria-label={importSource === 'csv' ? 'Perfil CSV' : 'Perfil de INI'}
              disabled={importSaving !== ''}
              onChange={(event) => {
                setImportProfile(event.target.value)
                setImportReport(null)
                setImportError('')
              }}
              value={importProfile}
            >
              <option value="">{importSource === 'csv' ? 'Selecione um perfil CSV' : 'Selecione um perfil de INI'}</option>
              {(importSource === 'csv' ? profiles : iniProfiles).map((profile) => <option key={profile.name} value={profile.name}>{profile.name}</option>)}
            </select>
            {importSource === 'csv' && <label className="existing-import__file">
              <span>{importFile?.name ?? 'Selecionar CSV'}</span>
              <input accept=".csv,text/csv" disabled={importSaving !== ''} onChange={(event) => {
                setImportFile(event.target.files?.[0] ?? null)
                setImportReport(null)
                setImportError('')
              }} type="file" />
            </label>}
            <label className="import-option">
              <input checked={overwriteImport} disabled={importSaving !== ''} onChange={(event) => setOverwriteImport(event.target.checked)} type="checkbox" />
              <span>Sobrescrever rascunhos</span>
            </label>
            <button className="button button--secondary" disabled={importSaving !== '' || !importProfile || (importSource === 'csv' && !importFile)} type="submit">
              {importSaving === 'testing' ? 'Ensaiando...' : 'Ensaiar importação'}
            </button>
          </div>
          {importError && <p className="form-error">{importError}</p>}
          {profiles.length === 0 && !importError && <p className="form-hint">Crie um perfil CSV em Configurar para importar.</p>}
          {importReport && (
            <div className="import-report">
              <h2>Ensaio da importação</h2>
              <div className="import-report__stats">
                <span><strong>{importReport.rows}</strong> {importSource === 'csv' ? 'linhas lidas' : 'imagens com INI'}</span>
                <span><strong>{importReport.matched}</strong> imagens encontradas</span>
                <span><strong>{importReport.applied}</strong> alterações previstas</span>
                <span><strong>{importReport.not_found_count}</strong> não encontradas</span>
              </div>
              <ImportWarnings report={importReport} />
              <button className="button button--primary" disabled={importSaving !== ''} onClick={() => void handleApplyImport()} type="button">
                {importSaving === 'applying' ? 'Aplicando...' : 'Aplicar importação'}
              </button>
            </div>
          )}
        </form>
        <BoxesSection packageName={selectedPackage.name} />
        <div className="detail-images__heading">
          <div>
            <h2>Imagens do pacote</h2>
            <p>{imageTotal.toLocaleString('pt-BR')} imagem(ns){images.length < imageTotal && ` · mostrando ${images.length.toLocaleString('pt-BR')}`}</p>
          </div>
          {iniProfiles.length > 0 && (
            <select
              aria-label="Perfil de INI no upload"
              className="upload-ini-select"
              disabled={saving}
              onChange={(event) => setUploadIniProfile(event.target.value)}
              title="Preenche o rascunho de cada imagem nova com o INI embutido"
              value={uploadIniProfile}
            >
              <option value="">Sem perfil de INI</option>
              {iniProfiles.map((profile) => <option key={profile.name} value={profile.name}>INI: {profile.name}</option>)}
            </select>
          )}
          <label className="button button--secondary upload-images-button">
            {saving ? 'Salvando...' : 'Adicionar imagens'}
            <input disabled={saving} onChange={handleAddImages} type="file" multiple {...{ webkitdirectory: 'true' }} />
          </label>
        </div>
        {upload && <UploadStatus progress={upload} />}
        {images.length > 0 ? (
          <div className="image-list">
            {images.map((image) => (
              <div className="image-list__item" key={image.sha1}>
                <span className="image-list__name" title={image.original_name}>{image.original_name}</span>
                <div className="image-list__actions">
                  <span className={image.status === 'verified' ? 'image-status image-status--verified' : 'image-status'}>
                    <i className={image.status === 'verified' ? 'status-dot status-dot--verified' : 'status-dot status-dot--pending'} />
                    {image.status === 'verified' ? 'Analisada' : 'Pendente'}
                  </span>
                  <button className="image-remove-button" disabled={saving} onClick={() => handleRemoveImage(image)} type="button">Remover</button>
                </div>
              </div>
            ))}
            {images.length < imageTotal && (
              <button className="button button--secondary image-list__more" disabled={saving} onClick={handleLoadMore} type="button">
                Carregar mais
              </button>
            )}
          </div>
        ) : (
          <p>Este pacote ainda não possui imagens carregadas.</p>
        )}
        {error && <p className="form-error">{error}</p>}
      </div>

      {showDeleteModal && (
        <div className="modal-backdrop" onClick={handleCloseDeleteModal} role="dialog" aria-modal="true">
          <div className="modal-dialog" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>Excluir pacote</h2>
              <button
                type="button"
                className="modal-close"
                onClick={handleCloseDeleteModal}
                disabled={saving}
                aria-label="Fechar"
              >
                ✕
              </button>
            </div>
            <form onSubmit={handleConfirmDelete}>
              <div className="modal-body">
                <p className="modal-warning">
                  Esta ação vai <strong>apagar permanentemente todas as imagens</strong> e o histórico deste pacote. Esta ação não pode ser desfeita.
                </p>
                <label className="form-field">
                  <span>Digite o nome do pacote <code>{selectedPackage.name}</code> para confirmar:</span>
                  <input
                    autoFocus
                    type="text"
                    value={deleteConfirmationInput}
                    onChange={(e) => setDeleteConfirmationInput(e.target.value)}
                    placeholder={selectedPackage.name}
                    disabled={saving}
                  />
                </label>
                {error && <p className="form-error">{error}</p>}
              </div>
              <div className="modal-actions">
                <button
                  type="button"
                  className="button button--secondary"
                  onClick={handleCloseDeleteModal}
                  disabled={saving}
                >
                  Cancelar
                </button>
                <button
                  type="submit"
                  className="button button--danger"
                  disabled={saving || deleteConfirmationInput.trim() !== selectedPackage.name}
                >
                  {saving ? 'Excluindo...' : 'Confirmar exclusão'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </section>
  )
}
