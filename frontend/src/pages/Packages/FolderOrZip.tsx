import { useEffect, useRef, useState, type ChangeEvent, type ReactNode } from 'react'

// Um botão só para "pasta ou zip". O navegador não deixa o mesmo seletor
// escolher pasta E arquivo, então o clique abre as duas opções.
export function FolderOrZip({ className, disabled, children, onFolderClick, onFolderChange, folderRef, onZipChange }: {
  className: string
  disabled?: boolean
  children: ReactNode // o conteúdo do botão
  onFolderClick: () => void // começou a ler a pasta (useFolderReading)
  onFolderChange: (event: ChangeEvent<HTMLInputElement>) => void
  folderRef: (input: HTMLInputElement | null) => void
  onZipChange: (event: ChangeEvent<HTMLInputElement>) => void
}) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const folder = useRef<HTMLInputElement | null>(null)
  const zip = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!open) return
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent ? event.key === 'Escape' : !box.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', close)
    }
  }, [open])

  function pick(input: HTMLInputElement | null) {
    setOpen(false)
    input?.click()
  }

  return (
    <div className="folder-or-zip" ref={box}>
      <button aria-expanded={open} className={className} disabled={disabled} onClick={() => setOpen((v) => !v)} type="button">
        {children}
      </button>
      {open && (
        <div className="folder-or-zip__menu" role="menu">
          <button onClick={() => pick(folder.current)} role="menuitem" type="button">
            <strong>Pasta de imagens</strong>
            <small>Escolhe a pasta; o navegador lê os arquivos</small>
          </button>
          <button onClick={() => pick(zip.current)} role="menuitem" type="button">
            <strong>Arquivo .zip</strong>
            <small>Para pastas grandes: vai em partes e continua de onde parou</small>
          </button>
        </div>
      )}
      <input
        hidden
        multiple
        onChange={onFolderChange}
        onClick={onFolderClick}
        ref={(input) => { folder.current = input; folderRef(input) }}
        type="file"
        {...{ webkitdirectory: 'true' }}
      />
      <input accept=".zip,application/zip" hidden onChange={onZipChange} ref={zip} type="file" />
    </div>
  )
}
