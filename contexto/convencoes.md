# Convenções

## Go (backend)

- **Português** nos comentários, nas mensagens de erro e nos nomes de teste
  (`TestPacoteComPerfilProprio`). Identificadores em inglês (`FindProfile`, `ImageFilter`).
- **Comentário explica o porquê**, não repete o código. Todo pacote tem um doc no
  arquivo principal dizendo o que é e quais coleções são dele.
- **Mensagens de erro para o usuário** dizem o que fazer: `"pacote %q não existe: crie
  antes com POST /packages"`.
- **Validação junta todos os erros** de uma vez quando a pessoa vai corrigir um formulário
  ou JSON (ex.: `annotation.InvalidError{Problems}` com o caminho de cada campo).
- **JSON de entrada com `DisallowUnknownFields`:** um campo com nome errado vira erro.
- **Limite de tamanho** em todo corpo de requisição (`http.MaxBytesReader`).
- **Listas paginadas** (`limit`, `skip`) respondem `platform.Page[T]` (`{"total", "items"}`).
- Formatação: `gofmt` e `goimports`. `go vet ./...` sem avisos.

## Testes

- `go test ./... -cover` em `backend/`. Com o Mongo do `docker-compose` de pé, os
  `mongo_test.go` rodam num banco descartável (`sidm_test_<n>`, apagado no fim) usando o
  root do `.env`; sem Mongo, eles são pulados (`t.Skip`), não falham.
- **Regras** se testam no `service_test.go` com o `Memory` do módulo. **Fluxos HTTP de
  ponta a ponta** estão em `internal/api/*_test.go` (o `newTestEnv` monta todos os módulos
  em memória e uma pasta temporária de imagens).
- O `-cover` de um pacote só conta os testes dele; para ver a cobertura que a API dá:
  `go test ./internal/api -coverpkg=./internal/images,./internal/exports`.
- Imagens de teste são geradas (`realJPEG`); nunca use foto real de placa.
- Todo bug corrigido ganha um teste que falharia sem a correção.

## Frontend

- `npm run dev` (porta 5173), `npm run build` (inclui `tsc -b`), `npm run lint`.
- Toda chamada à API passa por `services/api.ts`. Erro de validação chega como
  `RequestError` com `problems` (caminho + mensagem) para marcar o campo.
- Telas montadas a partir do perfil de anotação (nada de lista fixa de campos na anotação).
- Datas: a API usa UTC (ISO 8601); o `<input type="datetime-local">` usa hora local;
  converta com `utils/datetime.ts`.
- **Alertas são balões** (canto inferior direito, somem em 3 s; `services/toast.ts` +
  `<Toaster />` no layout): o erro/mensagem de uma tela é `const [error, setError] =
  useToastState('error')` (`hooks/useToast.ts`), e cada `setError(msg)` mostra o balão;
  fora de estado, `notify(texto, tipo)`. Fica fixo na tela só o que é estado permanente:
  validação de campo, aviso de confirmação no modal, "não existe mais", lista de problemas,
  erro no lugar do conteúdo que não carregou.
- **Confirmação é modal**, nunca `window.confirm`/`alert`: `if (!(await confirmDialog({
  title, message, confirmLabel, tone: 'danger' })))` (`services/confirm.ts`; o
  `<ConfirmDialog />` fica no layout). `tone: 'danger'` para o que apaga ou não se desfaz.
- A anotação usa a **fila** (`POST /packages/{nome}/next`), nunca a lista por índice.

- **Visual:** tema escuro (padrão) e claro, com os mesmos tokens, em `src/styles/theme.css`
  (o claro vale com `data-theme="light"` no `<html>`; `services/theme.ts` guarda a escolha
  no navegador, sem escolha segue o sistema; botão claro/escuro no header) (tokens: `--bg`, `--surface*`,
  `--border*`, `--text*`, `--accent*`, `--success/warning/danger*`, fontes `--font` e
  `--font-display`). CSS de tela **não escreve cor fixa (hex)**: usa os tokens. Botões,
  títulos e rótulos comuns ficam em `src/styles/components.css` (`.button`,
  `.button--primary/secondary/danger/sm`, `.eyebrow`, `.card`, `.form-error`). As cores das
  caixas em cima da foto (`utils/boxes.ts`) são a exceção.
- Fontes pelo npm (`@fontsource-variable/figtree` e `inter-tight`), não por CDN: a rede
  interna pode não alcançar o Google Fonts.

## Git

- Branch por assunto: `feat/...`, `fix/...`, `refactor/...`. Merge na `main` quando estável.
- Commits no estilo Conventional Commits, em português: `feat: perfil de anotação no
  pacote`, `fix: ...`, `refactor: módulo images`, `docs: ...`.
- Um commit = um passo testado. Rode os testes antes.
- O Windows mostra arquivos como modificados só por fim de linha (CRLF); confira com
  `git diff --ignore-cr-at-eol --stat`.

## Banco: backup antes de mexer nos dados

```powershell
docker exec sidm-mongo sh -c 'mongodump --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --db sidm --out /data/backup-<nome>'
docker cp sidm-mongo:/data/backup-<nome> ./backup-<nome>     # e ponha no .gitignore
```

As aspas simples fazem o shell do container expandir as variáveis: a senha não aparece no
terminal. Para voltar: `mongorestore --drop` com as mesmas opções.

## Rodando localmente

1. `docker compose up -d mongo` (só o Mongo; o serviço `backend` do compose sobe o código
   da imagem construída, que pode estar desatualizado).
2. `cd backend && go run ./cmd/server` (lê o `../.env`; porta 8080).
3. `cd frontend && npm run dev`.
