# SIDM — instruções para agentes de IA

Controle e anotação de imagens para datasets de visão computacional (Go + MongoDB + React).
Uso interno da Splice. **Responda e escreva em português** (código comentado, mensagens,
commits).

**O contexto completo está em [`contexto/`](contexto/README.md).** Antes de mudar código,
leia pelo menos `contexto/regras.md` e `contexto/arquitetura.md`. Se o pedido mexe em
dados, leia `contexto/dados.md`; para saber o que está em andamento, `contexto/estado.md`.

## O essencial (detalhes em `contexto/regras.md`)

- Nunca leia, imprima ou grave chaves privadas, senhas ou o `.env`. PGP não é usado.
- O arquivo original da imagem **nunca** é alterado (o INI embutido é assinado). No disco
  toda imagem é `dataset/<pacote>/<sha1>.ext`; o nome v2 é só um dado no banco.
- A `Assinatura` do INI não vai para o banco. Foto real de placa não vira fixture de teste.
- Backend é um **monólito modular** (`backend/internal/<módulo>/`): cada módulo é dono das
  suas coleções; os outros falam com ele pelo `Service`, por interfaces pequenas.
- O que se anota vem do **perfil de anotação do pacote** (`fields`/`objects` na imagem);
  não escreva listas fixas de campos.
- Rota nova = documentar no `backend/internal/api/openapi.yaml` (há teste que confere).
- Um passo de cada vez, com teste. Não faça commit/push sem a pessoa pedir; nunca na `main`.

## Comandos

```bash
cd backend && go vet ./... && go test ./... -cover   # com o Mongo do compose de pé, roda a integração
cd backend && go run ./cmd/server                    # porta 8080, lê ../.env
cd frontend && npm run dev                           # porta 5173, proxy /api -> 8080
cd frontend && npm run build && npm run lint
```

Ao terminar uma mudança que altera o desenho, atualize `contexto/` no mesmo commit
(`decisoes.md` para decisões, `estado.md` para o andamento).
