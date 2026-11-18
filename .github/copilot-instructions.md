# Instruções do projeto SIDM

As instruções completas ficam em `AGENTS.md` (raiz) e na pasta `contexto/`. Leia
`AGENTS.md` e `contexto/regras.md` antes de sugerir mudanças. Resumo:

- Responda e escreva em português (comentários, mensagens de erro, commits).
- Nunca exponha chaves, senhas ou o `.env`.
- O arquivo original da imagem nunca é alterado; no disco ela é sempre `<sha1>.ext`.
- Backend: monólito modular em `backend/internal/<módulo>/` (tipos em `<módulo>.go`,
  regras em `service.go`, rotas em `http.go`, Mongo em `mongo.go`, fake em `memory.go`).
- Os campos de anotação vêm do perfil do pacote (`fields`/`objects`); nada de lista fixa.
- Rota nova vai também no `backend/internal/api/openapi.yaml`.
- Frontend: toda chamada HTTP passa por `frontend/src/services/api.ts`.
