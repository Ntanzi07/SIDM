import { useNavigate, useParams } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { deletePackage, getPackage, imageFiles, listAnnotationProfiles, listImportProfiles, listIniProfiles, setPackageProfile, uploadImages, type ImportProfile, type PackageSummary, type UploadProgress } from '../../services/api'
import type { AnnotationProfile } from '../../types/annotation'
import type { ChangeEvent, FormEvent } from 'react'
import { useFolderReading } from '../../hooks/useFolderReading'
import { UploadStatus } from './UploadStatus'
import { BoxesSection } from './BoxesSection'
import { ImportModal, type ImportSource } from './ImportModal'
import { PackageImages } from './PackageImages'
import { defaultIniProfile, forPackage, optionLabel } from '../../utils/importProfiles'
import './Packages.css'
import { useIsAdmin } from '../../services/user'

const fmt = (n: number) => n.toLocaleString('pt-BR')

// Um pacote: os números em cima; embaixo, à esquerda (fixa) as caixas e os
// imports, e à direita as imagens em rolagem infinita.
export function PackageDetailPage() {
  const navigate = useNavigate()
  const admin = useIsAdmin() // apagar é só de admin
  const { id } = useParams()
  const [selectedPackage, setSelectedPackage] = useState<PackageSummary | undefined>()
  const [upload, setUpload] = useState<UploadProgress | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [csvProfiles, setCsvProfiles] = useState<ImportProfile[]>([])
  const [iniProfiles, setIniProfiles] = useState<ImportProfile[]>([])
  const [annotationProfiles, setAnnotationProfiles] = useState<AnnotationProfile[]>([])
  const [profileError, setProfileError] = useState('')
  const [uploadIniProfile, setUploadIniProfile] = useState('')
  const [importing, setImporting] = useState<ImportSource | null>(null) // o pop-up aberto
  const [imagesKey, setImagesKey] = useState(0) // muda = a lista de imagens recomeça
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [deleteConfirmationInput, setDeleteConfirmationInput] = useState('')
  const { reading: readingFolder, start: startReadingFolder, done: doneReadingFolder, inputRef: folderInputRef } = useFolderReading()

  const packageName = decodeURIComponent(id ?? '')

  async function refreshSummary() {
    setSelectedPackage(await getPackage(packageName))
  }

  useEffect(() => {
    let active = true
    getPackage(packageName)
      .then((summary) => { if (active) setSelectedPackage(summary) })
      .catch((requestError: unknown) => { if (active) setError(requestError instanceof Error ? requestError.message : 'Não foi possível carregar o pacote.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [packageName])

  useEffect(() => {
    listAnnotationProfiles().then(setAnnotationProfiles).catch(() => { /* sem a lista: só mostra o nome */ })
    listIniProfiles().then((list) => {
      setIniProfiles(list)
      setUploadIniProfile((current) => current || defaultIniProfile(list))
    }).catch(() => { /* sem perfis de INI: a opção some */ })
    listImportProfiles().then(setCsvProfiles).catch(() => { /* sem perfis CSV: o pop-up avisa */ })
  }, [])

  // O perfil de anotação só pode ser trocado sem imagem verificada (o
  // backend confere de novo).
  async function handleProfileChange(profile: string) {
    if (!selectedPackage || profile === selectedPackage.profile) return
    setProfileError('')
    try {
      await setPackageProfile(selectedPackage.name, profile)
      await refreshSummary()
    } catch (requestError: unknown) {
      setProfileError(requestError instanceof Error ? requestError.message : 'Não foi possível trocar o perfil.')
    }
  }

  function handleAddImages(event: ChangeEvent<HTMLInputElement>) {
    doneReadingFolder()
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
      .then(async (result) => {
        if (result.errors.length > 0) setError(`${result.errors.length} imagem(ns) não subiram. Selecione de novo para reenviar: as que já subiram são puladas.`)
        await refreshSummary()
        setImagesKey((key) => key + 1)
      })
      .catch((requestError: unknown) => setError(requestError instanceof Error ? requestError.message : 'Não foi possível adicionar as imagens.'))
      .finally(() => setSaving(false))
  }

  function handleImported() {
    setImporting(null)
    void refreshSummary()
    setImagesKey((key) => key + 1)
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

  if (loading) {
    return <section className="package-page empty-package"><p>Carregando pacote...</p></section>
  }

  if (!selectedPackage) {
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

  const pkg = encodeURIComponent(selectedPackage.name)
  const uploadLabel = saving ? 'Enviando...' : readingFolder ? 'Lendo a pasta...' : 'Adicionar imagens'

  return (
    <section className="package-page">
      <button className="back-button" onClick={() => navigate('/pacotes')} type="button">
        ← Voltar para pacotes
      </button>
      <div className="page-heading detail-heading">
        <div>
          <h1>{selectedPackage.name}</h1>
          <p>
            criado por {selectedPackage.created_by} · perfil de anotação{' '}
            {selectedPackage.verified === 0 && annotationProfiles.length > 1 ? (
              <select
                aria-label="Perfil de anotação"
                className="detail-profile-select"
                onChange={(event) => void handleProfileChange(event.target.value)}
                value={selectedPackage.profile}
              >
                {annotationProfiles.map((profile) => <option key={profile.name} value={profile.name}>{profile.name}</option>)}
              </select>
            ) : (
              <strong title={selectedPackage.verified > 0 ? 'Com imagens verificadas, o perfil não muda mais' : undefined}>
                {selectedPackage.profile}
              </strong>
            )}
          </p>
          {profileError && <p className="form-error">{profileError}</p>}
        </div>
        <div className="detail-heading__actions">
          {admin && <button className="button button--danger" disabled={saving} onClick={handleOpenDeleteModal} type="button">Excluir pacote</button>}
          <button
            className="button button--secondary"
            disabled={selectedPackage.verified === 0}
            onClick={() => navigate(`/pacotes/${pkg}/verificadas`)}
            type="button"
          >
            Ver verificadas
          </button>
          <button className="button button--primary" onClick={() => navigate(`/pacotes/${pkg}/anotacao`)} type="button">Abrir anotação</button>
        </div>
      </div>
      <div className="detail-stats">
        <div><strong>{fmt(selectedPackage.total)}</strong><span>Imagens</span></div>
        <div><strong>{fmt(selectedPackage.verified)}</strong><span>Verificadas</span></div>
        <div><strong>{fmt(selectedPackage.pending)}</strong><span>Pendentes</span></div>
      </div>

      <div className="detail-layout">
        <aside className="detail-side">
          <BoxesSection packageName={selectedPackage.name} total={selectedPackage.total} />
          <section className="side-section">
            <div className="side-section__head">
              <h2>Importar anotações</h2>
            </div>
            <p className="side-section__hint">Preenche os rascunhos das imagens com dados que já existem, antes de verificar.</p>
            <div className="side-section__actions">
              <button className="button button--secondary button--sm" onClick={() => setImporting('csv')} type="button">Importar CSV</button>
              {iniProfiles.length > 0 && (
                <button className="button button--secondary button--sm" onClick={() => setImporting('ini')} type="button">Usar o INI das imagens</button>
              )}
            </div>
          </section>
        </aside>

        <div className="detail-main">
          <div className="detail-main__head">
            <div>
              <h2>Imagens</h2>
              <span>{fmt(selectedPackage.total)} no pacote</span>
            </div>
            <div className="detail-main__actions">
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
                  {forPackage(iniProfiles, selectedPackage.profile).map((profile) => (
                    <option key={profile.name} value={profile.name}>{optionLabel(profile, selectedPackage.profile, 'INI: ')}</option>
                  ))}
                </select>
              )}
              <label className={readingFolder ? 'button button--primary button--sm upload-images-button upload-images-button--reading' : 'button button--primary button--sm upload-images-button'}>
                {uploadLabel}
                <input
                  disabled={saving}
                  multiple
                  onChange={handleAddImages}
                  onClick={startReadingFolder}
                  ref={folderInputRef}
                  type="file"
                  {...{ webkitdirectory: 'true' }}
                />
              </label>
            </div>
          </div>
          {upload && <UploadStatus progress={upload} />}
          {error && <p className="form-error">{error}</p>}
          <PackageImages key={imagesKey} onRemoved={() => void refreshSummary()} packageName={selectedPackage.name} />
        </div>
      </div>

      {importing && (
        <ImportModal
          csvProfiles={csvProfiles}
          iniProfiles={iniProfiles}
          onApplied={handleImported}
          onClose={() => setImporting(null)}
          packageName={selectedPackage.name}
          packageProfile={selectedPackage.profile}
          source={importing}
        />
      )}

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
