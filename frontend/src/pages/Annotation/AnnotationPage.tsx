import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { BoxAttributes } from '../../components/BoxAttributes/BoxAttributes'
import { ImageWithBoxes } from '../../components/ImageWithBoxes/ImageWithBoxes'
import { ZoomView } from '../../components/ZoomView/ZoomView'
import { boxPending, type BoxRef, colorOf } from '../../utils/boxes'
import { describeBoxSets, useImageBoxes } from '../../hooks/useImageBoxes'
import { useAnnotator } from '../../hooks/useAnnotator'
import { fieldLabel, listObjectTypes, impliedKeys } from '../../utils/annotationForm'
import { FieldInput } from './AnnotationFields'
import { BoxLink } from './BoxLink'
import { linkedKeys, toObjectBox } from '../../utils/boxLinks'
import { Bone, Skeleton } from '../../components/Skeleton/Skeleton'
import { SkButtons, SkFields } from '../../components/Skeleton/Shapes'
import { useShortcuts } from '../../hooks/useShortcuts'
import { ShortcutsHelp } from './ShortcutsHelp'
import './Annotation.css'

function formatSize(bytes: number) {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024)).toLocaleString('pt-BR')} KB`
  return `${(bytes / 1024 / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 2 })} MB`
}

export function AnnotationPage() {
  const navigate = useNavigate()
  const { id } = useParams()
  const packageName = decodeURIComponent(id ?? '')
  const [searchParams] = useSearchParams()
  const annotator = useAnnotator(packageName, searchParams.get('imagem') ?? undefined)
  const { image, form, profile } = annotator

  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [showBoxes, setShowBoxes] = useState(true)
  const { sets: boxSets, reload: reloadBoxes } = useImageBoxes(annotator.image?.sha1)
  // A caixa selecionada (para os atributos), por imagem: trocar de imagem limpa.
  const [picked, setPicked] = useState<{ sha1: string; box: BoxRef } | null>(null)
  const selectedBox = picked && picked.sha1 === annotator.image?.sha1 ? picked.box : null
  const selectBox = (box: BoxRef | null) => setPicked(box && annotator.image ? { sha1: annotator.image.sha1, box } : null)

  // O erro do backend para um campo (pelo caminho: "fields.day_time", "objects[0].plate").
  const problemAt = (path: string) => annotator.problems.find((problem) => problem.path === path)?.msg
  const objectTypes = listObjectTypes(profile)
  const boxTypes = profile.objects.filter((type) => type.box)
  const countOf = (type: string) => form.objects.filter((object) => object.type === type).length
  // as caixas que já são de um objeto (índice do objeto → caixa)
  const links = linkedKeys(form.objects, boxSets)
  const takenBy = (index: number) => new Set([...links].filter(([i]) => i !== index).map(([, key]) => key))
  const showBox = (ref: BoxRef) => { setShowBoxes(true); selectBox(ref) }
  // o nome do objeto de cada caixa ligada (caixa → "Veículo 1")
  const objectName = (index: number) => {
    const object = form.objects[index]
    const type = objectTypes.find((t) => t.key === object.type)
    const number = form.objects.slice(0, index + 1).filter((o) => o.type === object.type).length
    return `${type ? fieldLabel(type) : object.type} ${number}`
  }
  const linkedNames = new Map([...links].map(([index, key]) => [key, objectName(index)]))
  // caixa solta que pode virar um objeto da lista (motorcycle → "+ Veículo desta caixa")
  const actionFor = (ref: BoxRef) => {
    const box = boxSets.find((set) => set.task === ref.task)?.boxes[ref.index]
    const type = box && objectTypes.find((t) => t.box_labels?.some((label) => label.label === box.label))
    if (!box || !type) return null
    const full = (type.max ?? 0) > 0 && countOf(type.key) >= (type.max ?? 0)
    return {
      label: `+ ${fieldLabel(type)} desta caixa`,
      disabled: full,
      run: () => { annotator.addObject(type.key, toObjectBox(ref.task, box), box.attributes); showBox(ref) },
    }
  }
  // Os problemas que não são de um campo só (ex.: "pelo menos 1 veículo").
  const generalProblems = annotator.problems.filter((problem) => problem.path === 'objects' || problem.path.endsWith('.type'))

  // Atalhos de teclado (os da pessoa; Atalhos, no menu da conta)
  const [showHelp, setShowHelp] = useState(false)
  const allBoxes = boxSets.flatMap((set) => set.boxes.map((_, index) => ({ task: set.task, index })))
  const stepBox = (delta: number) => {
    if (allBoxes.length === 0) return
    const at = selectedBox ? allBoxes.findIndex((b) => b.task === selectedBox.task && b.index === selectedBox.index) : -1
    showBox(allBoxes[(at + delta + allBoxes.length) % allBoxes.length])
  }
  const shortcutKeys = useShortcuts({
    'annotation.previous': () => { if (annotator.canGoBack && !annotator.saving) void annotator.previous() },
    'annotation.next': () => {
      if (annotator.saving || !image) return
      void (annotator.revisiting ? annotator.forward() : annotator.skip())
    },
    'annotation.save': () => { if (!annotator.saving && image) void annotator.save() },
    'annotation.add_object': () => {
      const type = objectTypes[0]
      if (type && !((type.max ?? 0) > 0 && countOf(type.key) >= (type.max ?? 0))) annotator.addObject(type.key)
    },
    'annotation.focus_first': () => {
      const rows = document.querySelectorAll('.vehicle-row')
      const scope = rows[rows.length - 1] ?? document.querySelector('.annotation-fields')
      scope?.querySelector<HTMLElement>('.annotation-fields input:not(:disabled), .annotation-fields select:not(:disabled)')?.focus()
    },
    'annotation.toggle_boxes': () => setShowBoxes((value) => !value),
    'annotation.next_box': () => stepBox(1),
    'annotation.previous_box': () => stepBox(-1),
    'annotation.delete_image': () => { if (image) setShowDeleteModal(true) },
    'annotation.help': () => setShowHelp((value) => !value),
  }, !showDeleteModal)

  const { done, total } = annotator.progress
  const percent = total ? Math.round((done / total) * 100) : 0

  async function handleDeleteImage() {
    const ok = await annotator.deleteCurrentImage()
    if (ok) setShowDeleteModal(false)
  }

  // a silhueta da anotação: o progresso, a imagem e o formulário
  if (annotator.loading) return (
    <section className="annotation-page">
      <button className="back-button" onClick={() => navigate(`/pacotes/${encodeURIComponent(packageName)}`)} type="button">← Voltar para {packageName}</button>
      <Skeleton label="Carregando a fila">
        <div className="page-heading annotation-heading">
          <div className="sk-title">
            <Bone h={10} w={180} />
            <Bone h={40} r={8} w={300} />
            <Bone h={4} r={999} w={360} />
          </div>
          <SkButtons widths={[96, 112, 150, 92]} />
        </div>
        <div className="annotation-layout">
          <main className="annotation-main">
            <div className="annotation-workspace">
              <div className="annotation-preview-column">
                <div className="annotation-preview-card">
                  <div className="annotation-image-meta"><Bone h={12} w={240} /></div>
                  <div className="annotation-preview"><Bone h={360} r={8} w="100%" /></div>
                </div>
              </div>
              <div className="annotation-form-card">
                <div className="annotation-form-heading"><Bone h={20} w={170} /></div>
                <SkFields columns={2} count={8} />
              </div>
            </div>
          </main>
        </div>
      </Skeleton>
    </section>
  )
  if (annotator.error && !image && !annotator.finished) {
    return (
      <section className="annotation-page annotation-empty">
        <h1>Não foi possível abrir a anotação</h1>
        <p>{annotator.error}</p>
        <button className="button button--secondary" onClick={() => navigate(`/pacotes/${encodeURIComponent(packageName)}`)} type="button">Voltar</button>
      </section>
    )
  }

  return (
    <section className="annotation-page">
      {/* Delete image confirmation modal */}
      {showHelp && <ShortcutsHelp keys={shortcutKeys} onClose={() => setShowHelp(false)} />}
      {showDeleteModal && (
        <div className="modal-backdrop" onClick={() => setShowDeleteModal(false)}>
          <div className="modal-dialog" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <strong>Excluir imagem</strong>
              <button className="modal-close" onClick={() => setShowDeleteModal(false)} type="button">✕</button>
            </div>
            <div className="modal-body">
              <p className="modal-warning">Esta ação vai excluir permanentemente a imagem <strong>{image?.original_name}</strong> e todos os seus dados de anotação. Esta ação não pode ser desfeita.</p>
            </div>
            <div className="modal-actions">
              <button className="button button--secondary" onClick={() => setShowDeleteModal(false)} type="button">Cancelar</button>
              <button className="button button--danger" disabled={annotator.saving} onClick={() => void handleDeleteImage()} type="button">
                {annotator.saving ? 'Excluindo...' : 'Excluir imagem'}
              </button>
            </div>
          </div>
        </div>
      )}

      <button className="back-button" onClick={() => navigate(`/pacotes/${encodeURIComponent(packageName)}`)} type="button">← Voltar para {packageName}</button>
      <div className="page-heading annotation-heading">
        <div>
          <span className="eyebrow">{packageName} · anotação</span>
          <h1>
            {done.toLocaleString('pt-BR')} <small>de {total.toLocaleString('pt-BR')} concluídas · {percent}%</small>
          </h1>
          <span className="progress-line"><span style={{ width: `${percent}%` }} /></span>
        </div>
        <div className="annotation-navigation">
          <button className="button button--secondary button--sm annotation-shortcuts" onClick={() => setShowHelp(true)} title="Atalhos de teclado (?)" type="button">⌨ Atalhos</button>
          <button className="button button--secondary" disabled={!annotator.canGoBack || annotator.saving} onClick={() => void annotator.previous()} type="button">← Anterior</button>
          <span className="annotation-place">
            {image && annotator.place && (
              <strong aria-label={`Imagem ${annotator.place.position} de ${annotator.place.total} do pacote`} title="Número da imagem no pacote, pela ordem de chegada">
                Imagem {annotator.place.position.toLocaleString('pt-BR')} / {annotator.place.total.toLocaleString('pt-BR')}
              </strong>
            )}
            <small>
              {annotator.revisiting
                ? 'Revendo uma anterior'
                : `${annotator.progress.pending.toLocaleString('pt-BR')} pendente(s) na fila`}
            </small>
          </span>
          {annotator.revisiting ? (
            <button className="button button--secondary" disabled={annotator.saving} onClick={() => void annotator.forward()} type="button">Seguinte →</button>
          ) : (
            <button
              className="button button--secondary"
              disabled={!image || annotator.saving}
              onClick={() => void annotator.skip()}
              title="A imagem volta para o fim da fila"
              type="button"
            >
              Pular →
            </button>
          )}
        </div>
      </div>

      <div className="annotation-layout">
        <main className="annotation-main">
          {image ? (
            <div className="annotation-workspace">
              <div className="annotation-preview-column">
                <div className="annotation-preview-card">
                  <div className="annotation-image-meta">
                    <div>
                      <strong>{image.original_name}</strong>
                      <span>
                        {formatSize(image.size)} · {image.ext}
                        {boxSets.length > 0 && (
                          <label className="annotation-boxes-toggle" title="Caixas, por tarefa">
                            <input checked={showBoxes} onChange={(event) => setShowBoxes(event.target.checked)} type="checkbox" />
                            {describeBoxSets(boxSets)}
                          </label>
                        )}
                      </span>
                    </div>
                    <button
                      className="button button--danger button--sm"
                      disabled={annotator.saving}
                      onClick={() => setShowDeleteModal(true)}
                      type="button"
                    >
                      Excluir imagem
                    </button>
                  </div>
                  <div className="annotation-preview">
                    <ZoomView resetKey={image.sha1}>
                      <ImageWithBoxes
                        alt={image.original_name}
                        onSelect={(box) => { setShowBoxes(true); selectBox(box) }}
                        pending={boxPending(profile, boxSets)}
                        selected={selectedBox}
                        sets={boxSets}
                        sha1={image.sha1}
                        show={showBoxes}
                      />
                    </ZoomView>
                  </div>
                </div>
              </div>

              <div className="annotation-form-card">
                <div className="annotation-form-heading">
                  <div>
                    <h2>Dados da anotação</h2>
                    <p>Preencha os campos de acordo com o que aparece na imagem.</p>
                  </div>
                  {/* pendente é o normal desta tela; só avisa ao rever uma já concluída */}
                  {image.status === 'verified' && <span className="annotation-status annotation-status--done">Concluída</span>}
                </div>


                <div className="annotation-fields">
                  {profile.fields.map((field) => (
                    <FieldInput
                      error={problemAt(`fields.${field.key}`)}
                      field={field}
                      key={field.key}
                      onChange={(value) => annotator.updateField(field.key, value)}
                      value={form.fields[field.key] ?? ''}
                    />
                  ))}
                </div>

                <p className="annotation-hint">
                  Perfil <strong>{profile.name}</strong>.
                  {profile.fields.some((field) => field.required) || profile.objects.some((type) => (type.min ?? 0) > 0)
                    ? ' Os campos com * são obrigatórios para verificar; o resto pode ficar vazio (ou "Não informado").'
                    : ' Nenhum campo é obrigatório: deixe vazio (ou "Não informado") o que a imagem não tem ou não dá para conferir.'}
                  {profile.fields.some((field) => field.type === 'ref') && (
                    <> Equipamento ou câmera novo: digite o nome e salve; a tela pede para confirmar o cadastro.
                      Para renomear ou desativar, use <Link to="/configurar">Configurar</Link>.</>
                  )}
                </p>

                {form.objects.map((object, index) => {
                  const type = objectTypes.find((t) => t.key === object.type)
                  if (!type) return null
                  const number = form.objects.slice(0, index + 1).filter((o) => o.type === object.type).length
                  return (
                    // ligado a uma caixa: a cor dela na borda, como nos cards das caixas
                    <div className={links.has(index) ? 'vehicle-row vehicle-row--boxed' : 'vehicle-row'} key={index} style={links.has(index) && object.box ? { borderLeftColor: colorOf(object.box.label, object.box.task) } : undefined}>
                      <div className="vehicle-row-header">
                        <h3>{fieldLabel(type)} {number}</h3>
                        <button
                          className="button button--danger button--sm"
                          onClick={() => annotator.removeObject(index)}
                          type="button"
                        >
                          Remover
                        </button>
                      </div>
                      {(type.box_labels?.length ?? 0) > 0 && (
                        <BoxLink
                          object={object}
                          onLink={(box, attributes) => annotator.linkBox(index, box, attributes)}
                          onShow={showBox}
                          sets={boxSets}
                          taken={takenBy(index)}
                          type={type}
                        />
                      )}
                      {problemAt(`objects[${index}].box`) && <small className="annotation-field__error">{problemAt(`objects[${index}].box`)}</small>}
                      <div className="annotation-fields">
                        {type.fields.map((field) => (
                          <FieldInput
                            // o que a caixa ligada já diz fica travado (troca a caixa para mudar)
                            disabled={impliedKeys(type, object).includes(field.key)}
                            error={problemAt(`objects[${index}].${field.key}`)}
                            field={field}
                            key={field.key}
                            onChange={(value) => annotator.updateObject(index, field.key, value)}
                            value={object.values[field.key] ?? ''}
                          />
                        ))}
                      </div>
                    </div>
                  )
                })}

                {objectTypes.map((type) => {
                  const full = (type.max ?? 0) > 0 && countOf(type.key) >= (type.max ?? 0)
                  return (
                    <div className="annotation-object-type" key={type.key}>
                      {countOf(type.key) === 0 && (
                        <p className="annotation-hint">
                          Nenhum(a) {fieldLabel(type).toLowerCase()} nesta imagem.
                          {(type.min ?? 0) > 0 && ` Para verificar, são pelo menos ${type.min}.`}
                        </p>
                      )}
                      <button
                        className="button button--secondary annotation-add-vehicle"
                        disabled={full}
                        onClick={() => annotator.addObject(type.key)}
                        title={full ? `No máximo ${type.max} por imagem` : undefined}
                        type="button"
                      >
                        + Adicionar {fieldLabel(type).toLowerCase()}
                      </button>
                    </div>
                  )
                })}

                {boxTypes.length > 0 && !boxSets.some((set) => set.boxes.length > 0) && (
                  <p className="annotation-hint">
                    {boxTypes.map((type) => fieldLabel(type)).join(', ')}: rotulado(s) com caixas (Tarefas). Esta imagem ainda não tem caixas.
                  </p>
                )}

                {boxSets.some((set) => set.boxes.length > 0) && (
                  <BoxAttributes actionFor={actionFor} linked={linkedNames} onSaved={reloadBoxes} onSelect={(ref) => (ref ? showBox(ref) : selectBox(null))} profile={profile} selected={selectedBox} sets={boxSets} />
                )}

                {generalProblems.length > 0 && (
                  <ul className="annotation-feedback annotation-feedback--error annotation-problems">
                    {generalProblems.map((problem) => <li key={problem.path + problem.msg}>{problem.msg}</li>)}
                  </ul>
                )}

                <button className="button button--primary annotation-save" disabled={annotator.saving} onClick={() => void annotator.save()} type="button">
                  {annotator.saving ? 'Salvando...' : 'Salvar anotação'}
                </button>
              </div>
            </div>
          ) : (
            <div className="annotation-empty">
              <h2>Nada pendente na fila</h2>
              <p>
                {annotator.progress.total === 0
                  ? 'Este pacote ainda não tem imagens.'
                  : 'Todas as imagens livres já foram anotadas. Se alguém está com uma reservada, ela volta para a fila em até 15 minutos.'}
              </p>
            </div>
          )}
        </main>
      </div>
    </section>
  )
}
