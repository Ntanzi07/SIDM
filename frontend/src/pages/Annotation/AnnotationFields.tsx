import { useEffect, useState } from 'react'
import type { KeyboardEvent, ReactNode } from 'react'
import { searchCameras, searchEquipments, type PageResponse } from '../../services/api'
import type { FormValue, ProfileField } from '../../types/annotation'
import { fieldLabel } from '../../utils/annotationForm'

// Os controles do formulário de anotação, um por tipo de campo do perfil.
// Todo campo pode ficar "Não informado" (vazio): imagem sem o dado, ou que
// não dá para conferir. O que é obrigatório para verificar tem *.

type Wrap = {
  label: string
  required?: boolean
  error?: string
  help?: string
  invalid?: boolean
  unknown?: boolean
  className?: string
  group?: boolean // vários controles dentro (multi): <div>, porque <label> não pode ter <label> dentro
  children: ReactNode
}

function FieldWrap({ label, required, error, help, invalid, unknown, className, group, children }: Wrap) {
  const classes = ['annotation-field', className, invalid || error ? 'annotation-field--invalid' : '', unknown ? 'annotation-field--unknown' : '']
  const content = (
    <>
      <span>{label}{required && <em className="annotation-required" title="Obrigatório para verificar"> *</em>}</span>
      {children}
      {error && <small className="annotation-field__error">{error}</small>}
    </>
  )
  return group
    ? <div className={classes.filter(Boolean).join(' ')} role="group" title={help}>{content}</div>
    : <label className={classes.filter(Boolean).join(' ')} title={help}>{content}</label>
}

type SelectFieldProps = {
  label: string
  value: string
  options: readonly (readonly [string, string])[]
  onChange: (value: string) => void
  required?: boolean
  error?: string
  help?: string
}

function SelectField({ label, value, options, onChange, required, error, help }: SelectFieldProps) {
  // Valor que não está na lista (ex.: veio de um import sem tradução). Sem
  // isto, o navegador mostraria a primeira opção, e escolher essa opção não
  // dispararia mudança: o valor errado ficaria preso.
  const invalid = value !== '' && !options.some(([option]) => option === value)
  return (
    <FieldWrap error={error} help={help} invalid={invalid} label={label} required={required} unknown={value === ''}>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {invalid && <option value={value}>"{value}" (inválido: escolha outro)</option>}
        <option value="">Não informado</option>
        {options.map(([option, name]) => <option key={option} value={option}>{name}</option>)}
      </select>
    </FieldWrap>
  )
}

type AutocompleteFieldProps = {
  label: string
  value: string
  required?: boolean
  error?: string
  search: (q: string) => Promise<PageResponse<string>>
  placeholder?: string
  onChange: (value: string) => void
}

// Combobox próprio (o <datalist> do navegador não atualiza a lista aberta
// quando as sugestões chegam depois, e filtra demais quando já há um valor).
// Ao focar, mostra a lista toda; ao digitar, busca no backend (300 ms depois
// da última tecla). Setas + Enter escolhem, Esc fecha.
function AutocompleteField({ label, value, search, placeholder, onChange, required, error }: AutocompleteFieldProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [suggestions, setSuggestions] = useState<string[]>([])
  const [active, setActive] = useState(-1)

  useEffect(() => {
    if (!open) return
    let alive = true
    const timer = window.setTimeout(() => {
      search(query)
        .then((page) => {
          if (!alive) return
          setSuggestions(page.items)
          setActive(-1)
        })
        .catch(() => { if (alive) setSuggestions([]) })
    }, query ? 300 : 0)
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
  }, [open, query, search])

  function choose(name: string) {
    onChange(name)
    setOpen(false)
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setOpen(true)
      setActive((i) => Math.min(i + 1, suggestions.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (event.key === 'Enter' && open && active >= 0 && suggestions[active]) {
      event.preventDefault()
      choose(suggestions[active])
    } else if (event.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <FieldWrap className="annotation-combobox" error={error} label={label} required={required}>
      <input
        autoComplete="off"
        onBlur={() => setOpen(false)}
        onChange={(event) => {
          onChange(event.target.value)
          setQuery(event.target.value)
          setOpen(true)
        }}
        onFocus={() => {
          setQuery('') // ao entrar no campo, mostra todos (não só o que bate com o valor atual)
          setOpen(true)
        }}
        onKeyDown={handleKeyDown}
        placeholder={placeholder ?? 'Digite para buscar'}
        role="combobox"
        aria-expanded={open}
        value={value}
      />
      {open && suggestions.length > 0 && (
        <ul className="annotation-combobox__list" role="listbox">
          {suggestions.map((name, i) => (
            <li
              aria-selected={name === value}
              className={i === active ? 'annotation-combobox__item annotation-combobox__item--active' : 'annotation-combobox__item'}
              key={name}
              // mousedown (e não click): acontece antes do blur que fecha a lista
              onMouseDown={(event) => {
                event.preventDefault()
                choose(name)
              }}
              role="option"
            >
              {name}
            </li>
          ))}
        </ul>
      )}
      {open && query !== '' && suggestions.length === 0 && (
        <div className="annotation-combobox__list annotation-combobox__empty">
          Nenhum cadastrado com "{query}". Ao salvar, a tela pergunta se quer cadastrar.
        </div>
      )}
    </FieldWrap>
  )
}


type FieldInputProps = {
  field: ProfileField
  value: FormValue
  onChange: (value: FormValue) => void
  error?: string
}

// FieldInput: o controle de UM campo do perfil.
export function FieldInput({ field, value, onChange, error }: FieldInputProps) {
  const label = fieldLabel(field)
  const text = typeof value === 'string' ? value : ''
  const common = { error, help: field.help, label, required: field.required }

  switch (field.type) {
    case 'choice':
      return (
        <SelectField
          {...common}
          onChange={onChange}
          options={(field.options ?? []).map((option) => [option.value, option.label || option.value] as const)}
          value={text}
        />
      )
    case 'bool':
      return <SelectField {...common} onChange={onChange} options={[['true', 'Sim'], ['false', 'Não']]} value={text} />
    case 'multi': {
      const chosen = Array.isArray(value) ? value : []
      return (
        <FieldWrap {...common} className="annotation-field--wide" group unknown={chosen.length === 0}>
          <div className="annotation-multi">
            {(field.options ?? []).map((option) => (
              <label className={chosen.includes(option.value) ? 'annotation-chip annotation-chip--on' : 'annotation-chip'} key={option.value}>
                <input
                  checked={chosen.includes(option.value)}
                  onChange={(event) => onChange(event.target.checked
                    ? [...chosen, option.value]
                    : chosen.filter((v) => v !== option.value))}
                  type="checkbox"
                />
                {option.label || option.value}
              </label>
            ))}
          </div>
        </FieldWrap>
      )
    }
    case 'ref':
      return (
        <AutocompleteField
          error={error}
          label={label}
          onChange={onChange}
          placeholder={`Digite o ${field.ref === 'camera' ? 'nome da câmera' : 'equipamento'}`}
          required={field.required}
          search={field.ref === 'camera' ? searchCameras : searchEquipments}
          value={text}
        />
      )
    case 'datetime':
      return (
        <FieldWrap {...common} unknown={text === ''}>
          <input onChange={(event) => onChange(event.target.value)} step="1" type="datetime-local" value={text} />
        </FieldWrap>
      )
    case 'number':
      return (
        <FieldWrap {...common} unknown={text === ''}>
          <input
            inputMode={field.integer ? 'numeric' : 'decimal'}
            max={field.max}
            min={field.min}
            onChange={(event) => onChange(event.target.value)}
            step={field.integer ? 1 : 'any'}
            type="number"
            value={text}
          />
        </FieldWrap>
      )
    default: // text
      return (
        <FieldWrap {...common} unknown={text === ''}>
          <input
            maxLength={field.max_len || undefined}
            onChange={(event) => onChange(field.upper ? event.target.value.toUpperCase() : event.target.value)}
            placeholder={field.help || (field.required ? '' : 'Não sabe? Deixe vazio')}
            value={text}
          />
        </FieldWrap>
      )
  }
}
