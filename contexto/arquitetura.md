# Arquitetura

O backend é um **monólito modular**: um binário só, com cada assunto num pacote em
`backend/internal/<módulo>/`. Um módulo é dono das suas coleções do Mongo; os outros só
mexem nelas pelo `Service` dele.

## Os pacotes

| Pacote | Tipo | O que faz | Coleções |
|---|---|---|---|
| `catalog` | módulo | pacotes, equipamentos, câmeras | `packages`, `equipments`, `cameras` |
| `images` | módulo | upload, INI, fila, anotação, verificação, histórico, migrações | `images`, `image_history` |
| `annotation` | módulo | perfis de anotação (campos e tipos de objeto), herança (`extends`, `inherit.go`), caixa ligada a objeto (`box_labels`, `links.go`), `Check`/`Fit` dos valores | `annotation_profiles` |
| `boxes` | módulo | tarefas de anotação, caixas do CVAT (import COCO, `cvat.zip`) | `annotation_tasks`, `image_boxes` |
| `imports` | módulo | perfis de import, raio-x de arquivo, import de CSV/INI nos rascunhos | `import_profiles` |
| `exports` | módulo | perfis de export salvos, preview e download do zip | `export_profiles` |
| `versions` | módulo | datasets (pesquisas salvas) e versões congeladas deles, com o resumo para o dashboard | `datasets`, `versions`, `version_items` |
| `processing` | módulo | a página Processar: modelos de IA (adaptadores `Model`: LPR e eixos) passam pelas imagens dos pacotes escolhidos e gravam caixas numa tarefa pelo `boxes` (`PutModelSets`, que pula o que veio do CVAT ou foi editado); um processamento por modelo, em segundo plano | `processing_runs` |
| `users` | módulo | login: contas (admin/usuário, cadastro pendente até aprovar), sessões de 8 h, o middleware `Protect` (401/403) | `users`, `sessions` |
| `filename` | biblioteca pura | nome v2 (formatar e ler), tabelas de códigos, sanitização | — |
| `importer` | biblioteca pura | ler CSV e INI, traduzir com um perfil de import para os campos de um perfil de anotação (`annotation.Values`) | — |
| `export` | biblioteca pura | perfis de export, divisão train/valid/test, pastas e nomes pelos campos do perfil de anotação (`MetaFields`), escrita COCO/YOLO/VOC e só imagens (com `anotacoes.csv`) | — |
| `files` | biblioteca | disco: receber com hash, guardar, mover, apagar | — |
| `modelrpc` | biblioteca | conexão `net/rpc` (gob) comum aos servidores dos modelos de IA: prazo por chamada, reconectar, `Ping` | — |
| `lpr` | biblioteca | cliente do detector de placas `lpr-class` (`LprRPC.GetObjects`); os tipos copiam os do servidor, com os mesmos nomes de campo | — |
| `axle` | biblioteca | cliente do contador de eixos `axlecounter.software` (`AxleRPC.GetAxlesFull`); resposta zerada = servidor desligado (`ErrInactive`) | — |
| `crypt` | biblioteca | abrir imagens criptografadas dos equipamentos | — |
| `platform` | comum | erros (`ErrNotFound`, `ErrDuplicate`), `Route`, `Page[T]`, `WriteJSON`, `UserFrom`, conexão Mongo, `mongotest` | — |
| `api` | junção | junta as rotas dos módulos, `/health`, `/docs` | — |

"Biblioteca pura" = sem banco e sem HTTP: fácil de testar e reaproveitar. Lógica de
formato vai para a biblioteca; o que precisa de banco ou HTTP vai para o módulo.

## Os arquivos de um módulo

| Arquivo | O que tem |
|---|---|
| `<módulo>.go` | o doc do pacote, os tipos, as interfaces (`Repo` e o que ele usa dos outros) |
| `service.go` | as regras; recebe `ctx` e devolve `error`, **sem** `http.ResponseWriter` |
| `http.go` | `Routes() []platform.Route` e os handlers (decodificar, chamar o Service, traduzir o erro) |
| `mongo.go` | o `MongoRepo` (só as coleções do módulo); `EnsureIndexes` quando precisa |
| `memory.go` | o repositório em memória, com campos exportados, para os testes |
| `*_test.go` | testes do Service com `Memory`; `mongo_test.go` com Mongo de verdade |

Erros de regra viram tipos (`InvalidError{Msg}` → 400). `platform.ErrNotFound` → 404,
`platform.ErrDuplicate` → 409; o handler decide a mensagem.

## Como os módulos conversam

- **Por interfaces pequenas declaradas em quem USA** (ex.: `imports.Images` tem só os 5
  métodos que o import usa do `images.Service`). Nunca importe o `MongoRepo` de outro módulo.
- **Cascatas são avisos (listeners):** quem é dono do dado avisa quem guarda uma cópia.
  - `catalog` → `PackageListener` (renomear/apagar pacote): `boxes`.
  - `images` → `Listener` (apagar imagem): `boxes`.
  - `boxes` → `TaskListener` (renomear tarefa): `exports`.
- **Dependência circular se resolve com setter**, ligado no `main.go`:
  `images.SetCatalog`, `catalog.Listen`, `images.Listen`, `boxes.Listen`, `annotation.SetPackages`.
- **Sem transações** (o Mongo não tem replica set): as operações em várias coleções são
  ordenadas para que repetir o pedido termine o serviço (ex.: renomear equipamento devolve
  `complete: false` e o mesmo pedido continua de onde parou).

## Montagem (`backend/cmd/server/main.go`)

Ordem: conexão → `annotation` → migrações das imagens (`Migrate`, `EnsureIndexes`) →
`images` → `catalog` → migração dos arquivos (`MigrateFiles`) → `boxes` → `imports` →
`exports` → `versions` → `users` → `processing` (os modelos pelo ambiente: `LPR_ADDR`, `AXLE_ADDR`;
`MarkInterrupted` na subida) → `api.New(ping, módulos...)`. O `images` nasce antes do `imports` e busca o
perfil de INI do upload por `imports.Profiles{Repo}` (os padrão do código primeiro, depois o banco). Um módulo novo é criado ali e entra como mais um
argumento do `api.New`.

## Frontend (`frontend/src/`)

| Pasta | O que tem |
|---|---|
| `services/api.ts` | **todas** as chamadas HTTP (nenhuma tela usa `fetch` direto); `RequestError` com `problems` |
| `services/user.ts` | a conta logada (`useAccount`, `useIsAdmin`); a sessão é o cookie, o app pergunta o `/auth/me` |
| `app/auth/RequireLogin.tsx` | sem sessão, vai para `/login`; `RequireAdmin` guarda o Configurar e `/usuarios` |
| `hooks/` | estado e regras das telas (`useAnnotator`, `useImageBoxes`) |
| `types/` | os tipos da API (`annotation.ts` espelha o módulo `annotation`) |
| `utils/annotationForm.ts` | conversão API ↔ formulário dos campos do perfil |
| `utils/boxLinks.ts` | achar a caixa ligada a um objeto (posição/IoU) e as caixas que podem ser um objeto do tipo |
| `utils/boxes.ts` | caixas do CVAT: cor por label, tipo de objeto com caixa de uma label, pendência de atributo obrigatório |
| `utils/importProfiles.ts` | ordem e rótulo dos perfis de import nas telas de pacote (os do perfil do pacote primeiro; o `splice-ini` vem escolhido) |
| `utils/repeatFields.ts` | os valores da última imagem salva por pacote (campos com `repeat`), no `localStorage` |
| `pages/<Tela>/` | uma pasta por tela, com o `.tsx` e o `.css` (`pages/Process`: a página Processar) |
| `components/` | o que é usado por mais de uma tela (`ImageWithBoxes`: as caixas por cima da imagem; `BoxAttributes`: ver e editar os atributos das caixas) |

O proxy do Vite manda `/api` para `http://localhost:8080`.
