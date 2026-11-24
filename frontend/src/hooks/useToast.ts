import { useCallback, useEffect, useState } from 'react'
import { notify, type ToastKind } from '../services/toast'

// O erro ou a mensagem de uma tela como balão: um estado de texto que, a cada
// set com texto, mostra o balão (mesmo que o texto seja o de antes). O valor
// continua disponível para a tela (ex.: esconder o "Carregando..." com erro).
export function useToastState(kind: ToastKind): [string, (text: string) => void] {
  const [text, setText] = useState('')
  const set = useCallback((next: string) => {
    setText(next)
    if (next) notify(next, kind)
  }, [kind])
  return [text, set]
}

// Para um texto que não é um estado da tela (vem de outro hook, ou depende de
// outra condição): mostra o balão quando ele aparece ou muda.
export function useToast(text: string | null | undefined, kind: ToastKind = 'success') {
  useEffect(() => {
    if (text) notify(text, kind)
  }, [text, kind])
}
