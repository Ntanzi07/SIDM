# Frontend do SIDM

> Este arquivo substituiu o plano inicial, que descrevia um anotador só com
> localStorage, sem backend e com o nome de arquivo antigo. Nada disso vale
> mais: a fonte da verdade é a API (veja `/docs` no backend e o README da raiz).

## Como roda

```bash
npm install
npm run dev     # http://localhost:5173, com proxy /api -> http://localhost:8080
```

`VITE_API_URL` troca o endereço da API (padrão `/api`).

## Organização

| Pasta | O que tem |
|---|---|
| `services/api.ts` | todas as chamadas HTTP; nenhuma tela usa `fetch` direto |
| `services/user.ts` | o nome de quem está usando (vai no header `X-User`) |
| `hooks/useAnnotator.ts` | estado e regras da anotação |
| `data/annotationOptions.ts` | rótulos em português dos códigos do padrão |
| `utils/datetime.ts` | conversão entre a hora local do formulário e o UTC da API |
| `pages/` | uma pasta por tela |

## Regras que valem para o projeto todo

- **A API usa os VALORES, não as letras.** Direção é `rear`, não `T`; período
  é `night`, não `N`; resolução é `1920x1080`, não `08`. As letras só existem no
  nome do arquivo, e quem converte é o backend. `data/annotationOptions.ts` dá
  nome em português a esses valores, que precisam ser os mesmos de
  `backend/internal/filename/codes.go` (o lado direito de cada tabela, também no
  `GET /codes`). A tabela de resoluções vem direto do `GET /codes`.
- **A anotação usa a fila** (`POST /packages/{nome}/next`). Cada imagem fica
  reservada para quem pegou por 15 minutos; ao sair da tela, ela é devolvida
  (`/release`). Nunca liste todas as imagens para anotar por índice.
- **Datas:** a API usa UTC (ISO 8601). O `<input type="datetime-local">` usa a
  hora local. Converta com `utils/datetime.ts`, nunca com `slice` na string ISO.
- **Upload em lotes** (`uploadImages`): um pacote pode ter dezenas de milhares
  de imagens.
- **Listas grandes são paginadas** (`limit`/`skip`); autocomplete busca no
  backend com `?q=`.
- **Perfis de import:** a tela edita só a coluna de cada campo. Ao salvar, o
  que ela não mostra (`map`, `split`, `value`, `template`) é mantido.
- **Equipamento e câmera** não são cadastrados automaticamente na anotação:
  um erro de digitação viraria um cadastro novo. O cadastro fica em Configurar.
