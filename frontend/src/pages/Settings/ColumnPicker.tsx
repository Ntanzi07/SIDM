import { useState } from 'react'
import type { KeyboardEvent } from 'react'

type ColumnPickerProps = {
  value: string
  // As colunas do CSV / chaves do INI que podem ser escolhidas.
  columns: string[]
  onChange: (column: string) => void
  placeholder?: string
}

// Tira acento e caixa para a busca ("Data/Hora" acha "data").
const normalize = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

// ColumnPicker: escolhe UMA coluna da lista, buscando pelo texto. Não cria
// coluna: o que for digitado só filtra; sair do campo sem escolher volta ao
// valor de antes. A primeira opção, "Não mapear", limpa.
export function ColumnPicker({ value, columns, onChange, placeholder = 'Não mapear' }: ColumnPickerProps) {
  // text: o que está sendo digitado (null = não está editando, mostra o valor)
  const [text, setText] = useState<string | null>(null)
  const [active, setActive] = useState(0)

  const open = text !== null
  // Um valor salvo que estas imagens/este CSV não têm continua escolhível.
  const all = value && !columns.includes(value) ? [...columns, value] : columns
  const query = normalize(text ?? '')
  const matches = all.filter((column) => normalize(column).includes(query))
  // Sem busca, "Não mapear" vem primeiro; buscando, só as colunas.
  const options = query ? matches : ['', ...matches]

  function choose(column: string) {
    onChange(column)
    setText(null)
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      if (!open) setText('')
      setActive((i) => Math.min(i + 1, options.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (event.key === 'Enter') {
      // Enter não envia o formulário: escolhe a opção marcada.
      event.preventDefault()
      if (open && options[active] !== undefined) choose(options[active])
    } else if (event.key === 'Escape' && open) {
      event.preventDefault()
      setText(null)
    }
  }

  return (
    <div className="column-picker">
      <input
        aria-autocomplete="list"
        aria-expanded={open}
        autoComplete="off"
        onBlur={() => setText(null)}
        onChange={(event) => {
          setText(event.target.value)
          setActive(0)
        }}
        onFocus={(event) => {
          setText('') // ao entrar, mostra todas
          setActive(0)
          event.target.select()
        }}
        onKeyDown={handleKeyDown}
        placeholder={open && value ? value : placeholder}
        role="combobox"
        value={text ?? value}
      />
      {open && (
        <ul className="column-picker__list" role="listbox">
          {options.map((column, i) => (
            <li
              aria-selected={column === value}
              className={[
                'column-picker__item',
                i === active ? 'column-picker__item--active' : '',
                column === '' ? 'column-picker__item--none' : '',
              ].filter(Boolean).join(' ')}
              key={column || '__none'}
              // mousedown (e não click): acontece antes do blur que fecha a lista
              onMouseDown={(event) => {
                event.preventDefault()
                choose(column)
              }}
              onMouseEnter={() => setActive(i)}
              role="option"
            >
              {column || 'Não mapear'}
            </li>
          ))}
          {options.length === 0 && (
            <li className="column-picker__empty">Nenhuma chave com "{text}".</li>
          )}
        </ul>
      )}
    </div>
  )
}
