import { useCallback, useState } from 'react'

// O navegador lê a pasta inteira (milhares de arquivos levam segundos) ANTES
// de avisar a página: sem isto, parece que a seleção não foi. A página só
// sabe do clique (começou), do change (a pasta chegou) e do cancel (fechou a
// janela sem escolher): "reading" vale entre o clique e um dos dois.
//
// Uso: <input ref={inputRef} onClick={start} onChange={(e) => { done(); ... }} />
export function useFolderReading() {
  const [reading, setReading] = useState(false)
  const done = useCallback(() => setReading(false), [])
  const start = useCallback(() => setReading(true), [])
  // callback ref: o input pode aparecer depois (renderização condicional)
  const inputRef = useCallback((input: HTMLInputElement | null) => {
    if (!input) return
    input.addEventListener('cancel', done)
    return () => input.removeEventListener('cancel', done)
  }, [done])
  return { reading, start, done, inputRef }
}
