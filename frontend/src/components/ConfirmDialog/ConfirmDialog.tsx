import { useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { answerConfirm, subscribeConfirm, type ConfirmRequest } from '../../services/confirm'
import './ConfirmDialog.css'

// O modal das confirmações (services/confirm.ts). Fica no layout, uma vez só.
// Enter confirma, Esc ou o fundo cancelam; o foco vai para o botão de confirmar.
export function ConfirmDialog() {
  const [request, setRequest] = useState<ConfirmRequest | null>(null)
  useEffect(() => subscribeConfirm(setRequest), [])
  if (!request) return null
  return createPortal(<Dialog key={request.id} request={request} />, document.body)
}

function Dialog({ request }: { request: ConfirmRequest }) {
  const titleId = useId()
  const messageId = useId()
  const confirmRef = useRef<HTMLButtonElement>(null)
  const answer = (ok: boolean) => answerConfirm(request.id, ok)

  useEffect(() => {
    // devolve o foco para onde estava ao fechar (ex.: o campo da anotação)
    const previous = document.activeElement as HTMLElement | null
    confirmRef.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') answerConfirm(request.id, false)
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      previous?.focus?.()
    }
  }, [request.id])

  const danger = request.tone === 'danger'
  return (
    <div className="modal-backdrop" onClick={() => answer(false)}>
      <div
        aria-describedby={messageId}
        aria-labelledby={titleId}
        aria-modal="true"
        className="modal-dialog confirm-dialog"
        onClick={(event) => event.stopPropagation()}
        role="alertdialog"
      >
        <div className="confirm-dialog__body">
          <span aria-hidden="true" className={danger ? 'confirm-dialog__icon confirm-dialog__icon--danger' : 'confirm-dialog__icon'}>
            {danger ? '!' : '?'}
          </span>
          <div>
            <h2 id={titleId}>{request.title}</h2>
            <p id={messageId}>{request.message}</p>
          </div>
        </div>
        <div className="modal-actions">
          <button className="button button--secondary" onClick={() => answer(false)} type="button">
            {request.cancelLabel ?? 'Cancelar'}
          </button>
          <button
            className={danger ? 'button button--danger confirm-dialog__danger' : 'button button--primary'}
            onClick={() => answer(true)}
            ref={confirmRef}
            type="button"
          >
            {request.confirmLabel ?? 'Confirmar'}
          </button>
        </div>
      </div>
    </div>
  )
}
