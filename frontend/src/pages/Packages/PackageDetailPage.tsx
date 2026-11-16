import { useNavigate, useParams } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { addPackageImages, deleteImage, deletePackage, importPackageCsv, listImportProfiles, listPackageImages, listPackages, type ImageSummary, type ImportProfile, type ImportReport, type PackageSummary } from '../../services/api'
import type { ChangeEvent, FormEvent } from 'react'
import './Packages.css'

export function PackageDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams()
  const [selectedPackage, setSelectedPackage] = useState<PackageSummary | undefined>()
  const [images, setImages] = useState<ImageSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [profiles, setProfiles] = useState<ImportProfile[]>([])
  const [importProfile, setImportProfile] = useState('')
  const [importFile, setImportFile] = useState<File | null>(null)
  const [overwriteImport, setOverwriteImport] = useState(true)
  const [importReport, setImportReport] = useState<ImportReport | null>(null)
  const [importError, setImportError] = useState('')
  const [importSaving, setImportSaving] = useState<'testing' | 'applying' | ''>('')
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteConfirmationInput, setDeleteConfirmationInput] = useState('')

  useEffect(() => {
    const packageName = decodeURIComponent(id ?? '')
    Promise.all([listPackages(), listPackageImages(packageName)])
      .then(([packages, packageImages]) => {
        setSelectedPackage(packages.find((item) => item.name === packageName))
        setImages(packageImages)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar o pacote.'))
      .finally(() => setLoading(false))
  }, [id])

  useEffect(() => {
    listImportProfiles()
      .then(setProfiles)
      .catch((requestError: unknown) => setImportError(
        requestError instanceof Error ? requestError.message : 'Não foi possível carregar os perfis CSV.',
      ))
  }, [])

  function handleAddImages(event: ChangeEvent<HTMLInputElement>) {
    if (!event.target.files || !selectedPackage) return
    setSaving(true)
    setError('')
    addPackageImages(selectedPackage.name, event.target.files)
      .then(() => Promise.all([listPackageImages(selectedPackage.name), listPackages()]))
      .then(([packageImages, packages]) => {
        setImages(packageImages)
        setSelectedPackage(packages.find((item) => item.name === selectedPackage.name))
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível adicionar as imagens.'))
      .finally(() => {
        setSaving(false)
        event.target.value = ''
      })
  }

  function handleRemoveImage(image: ImageSummary) {
    if (!window.confirm(`Remover permanentemente a imagem "${image.original_name}"?`)) return
    setSaving(true)
    setError('')
    deleteImage(image.sha1)
      .then(() => Promise.all([listPackageImages(selectedPackage?.name ?? ''), listPackages()]))
      .then(([packageImages, packages]) => {
        setImages(packageImages)
        setSelectedPackage(packages.find((item) => item.name === selectedPackage?.name))
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
    if (!selectedPackage || !importProfile || !importFile) {
      setImportError('Selecione um perfil e um arquivo CSV.')
      return
    }

    setImportSaving('testing')
    setImportError('')
    try {
      const report = await importPackageCsv(selectedPackage.name, importProfile, importFile, true, overwriteImport)
      setImportReport(report)
    } catch (requestError: unknown) {
      setImportError(requestError instanceof Error ? requestError.message : 'Não foi possível ensaiar a importação.')
    } finally {
      setImportSaving('')
    }
  }

  async function handleApplyImport() {
    if (!selectedPackage || !importProfile || !importFile) return
    setImportSaving('applying')
    setImportError('')
    try {
      await importPackageCsv(selectedPackage.name, importProfile, importFile, false, overwriteImport)
      const [packageImages, packages] = await Promise.all([
        listPackageImages(selectedPackage.name),
        listPackages(),
      ])
      setImages(packageImages)
      setSelectedPackage(packages.find((item) => item.name === selectedPackage.name))
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
              <h2>Importar anotações CSV</h2>
              <p>Preencha os rascunhos deste pacote usando um perfil existente.</p>
            </div>
          </div>
          <div className="existing-import__controls">
            <select
              aria-label="Perfil CSV"
              disabled={importSaving !== ''}
              onChange={(event) => {
                setImportProfile(event.target.value)
                setImportReport(null)
                setImportError('')
              }}
              value={importProfile}
            >
              <option value="">Selecione um perfil CSV</option>
              {profiles.map((profile) => <option key={profile.name} value={profile.name}>{profile.name}</option>)}
            </select>
            <label className="existing-import__file">
              <span>{importFile?.name ?? 'Selecionar CSV'}</span>
              <input accept=".csv,text/csv" disabled={importSaving !== ''} onChange={(event) => {
                setImportFile(event.target.files?.[0] ?? null)
                setImportReport(null)
                setImportError('')
              }} type="file" />
            </label>
            <label className="import-option">
              <input checked={overwriteImport} disabled={importSaving !== ''} onChange={(event) => setOverwriteImport(event.target.checked)} type="checkbox" />
              <span>Sobrescrever rascunhos</span>
            </label>
            <button className="button button--secondary" disabled={importSaving !== '' || !importProfile || !importFile} type="submit">
              {importSaving === 'testing' ? 'Ensaiando...' : 'Ensaiar importação'}
            </button>
          </div>
          {importError && <p className="form-error">{importError}</p>}
          {profiles.length === 0 && !importError && <p className="form-hint">Crie um perfil CSV em Configurar para importar.</p>}
          {importReport && (
            <div className="import-report">
              <h2>Ensaio da importação</h2>
              <div className="import-report__stats">
                <span><strong>{importReport.rows}</strong> linhas lidas</span>
                <span><strong>{importReport.matched}</strong> imagens encontradas</span>
                <span><strong>{importReport.applied}</strong> alterações previstas</span>
                <span><strong>{importReport.not_found_count}</strong> não encontradas</span>
              </div>
              {Object.entries(importReport.unmapped).map(([field, values]) => (
                <p className="import-report__detail" key={field}>
                  <strong>{field}:</strong> valores sem tradução ({Object.keys(values).length})
                </p>
              ))}
              <button className="button button--primary" disabled={importSaving !== ''} onClick={() => void handleApplyImport()} type="button">
                {importSaving === 'applying' ? 'Aplicando...' : 'Aplicar importação'}
              </button>
            </div>
          )}
        </form>
        <div className="detail-images__heading">
          <div>
            <h2>Imagens do pacote</h2>
            <p>{images.length} imagem(ns) carregada(s)</p>
          </div>
          <label className="button button--secondary upload-images-button">
            {saving ? 'Salvando...' : 'Adicionar imagens'}
            <input accept="image/*" onChange={handleAddImages} type="file" multiple {...{ webkitdirectory: 'true' }} />
          </label>
        </div>
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
