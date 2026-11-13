# SIDM

Sistema de controle e anotação de imagens de veículos.

As imagens chegam em **pacotes**, sem padrão nenhum no nome. O time anota cada
imagem (equipamento, câmera, ângulo, clima, veículos, placas...), confere e
**verifica**. A imagem verificada ganha um nome autodescritivo no padrão v2, e
os metadados ficam no MongoDB, pesquisáveis por qualquer campo.

> **v1 interna:** não há login ainda. O usuário é informado no header `X-User`.
> Rode apenas na rede interna.

---

## Como funciona

```
 1. criar pacote         2. upload               3. fila de anotação              4. verificar
 POST /packages  ──▶  POST /images?package=X ──▶  POST /packages/X/next  ──▶  PUT /images/{sha1}
                      (pending, nome = hash)     (cada pessoa pega uma)       {"complete": true}
                                                                              (renomeia para v2)
```

- **Pacote:** um lote de imagens enviado junto. Cada pacote é uma pasta no servidor,
  e cada imagem pertence a **um** pacote só.
- **Duplicatas:** o SHA-1 do conteúdo é a identidade da imagem. Uma imagem que já
  existe em outro pacote é **pulada** no upload (`duplicate`, com o `existing_package`).
- **Rascunho:** `PUT` com `"complete": false` salva o que já foi preenchido, sem validar
  tudo e sem renomear. A imagem continua `pending`.
- **Verificar:** `PUT` com `"complete": true` valida tudo, confere o cadastro de
  equipamento e câmera, renomeia para o padrão v2 e marca `verified`.
- **Fila compartilhada:** várias pessoas anotam o mesmo pacote ao mesmo tempo, e cada
  uma pega "a próxima". Ninguém recebe a mesma imagem (veja [Fila de trabalho](#fila-de-trabalho)).
- **Histórico:** toda alteração de campo vai para `image_history` (quem, o quê, quando).

### Status de uma imagem

| Status | Nome do arquivo | Significado |
|---|---|---|
| `pending` | `<sha1>.jpg` | aguardando verificação (pode ter rascunho) |
| `verified` | `<sha1>_v2_..._.jpg` | anotada e conferida |

---

## Estrutura

```
SIDM/
├── .env / .env.example     portas, senhas do Mongo, pasta das imagens (.env fora do git)
├── docker-compose.yml      mongo + backend, healthchecks, redes separadas
├── mongo/init/
│   ├── 00-users.js         cria o usuário sidm_app (readWrite só no banco sidm)
│   └── 01-init.js          collections e índices
├── data/images/            (fora do git)
│   ├── tmp/                uploads em andamento
│   └── dataset/
│       ├── <pacote-1>/     <sha1>.jpg (pending) e <nome v2>.jpg (verified)
│       └── <pacote-2>/
├── frontend/               TODO
└── backend/                Go
    ├── cmd/server/         main: lê o .env, conecta no Mongo, sobe o HTTP
    └── internal/
        ├── filename/       padrão v2: Parse, Format, tabelas de códigos, sanitização
        ├── files/          disco: hash durante a gravação, pasta por pacote, rename
        ├── store/          Mongo: imagens, pacotes, fila, cadastros, histórico
        └── api/            handlers HTTP (dependem da interface Store)
```

---

## Padrão de nome v2

```
<sha1>_v2_<equipamento>_<câmera>_<dir-res-ângulo-período-alcance-clima>_<condições>_<data UTC>_<veículos>.jpg

4221aa851371c35af92bd2b241327bea11c70ba6_v2_8003-MF4739_hikvision-233_F-08-L-D-E-S_01_20260903T112523Z_ABC1D23-M-8-T-WHT.jpg
```

| Separador | Uso |
|---|---|
| `_` | separa os grupos (sempre 8) |
| `-` | separa os campos dentro de um grupo |
| `.` | separa os itens de uma lista (várias condições, vários veículos) |

- Cada veículo é `placa-tipoPlaca-categoria-tipoVeículo-cor`, por exemplo `ABC1D23-M-8-T-WHT`.
- A data é sempre em **UTC**. O front pode mandar com fuso (`-03:00`), e o servidor converte.
- Os **códigos fixos** (direção, resolução, clima, cor...) ficam em `backend/internal/filename/codes.go`.
  Um código existente **nunca** muda de significado: pode adicionar, nunca reaproveitar.
- **Equipamentos e câmeras** são cadastros no Mongo e entram no nome com o nome real sanitizado
  (espaço vira `-`; só `A-Z a-z 0-9 -`).
- O `v2` é a versão do padrão. Se um dia o padrão mudar, o parser lê as duas versões.

---

## API

| Rota | O que faz |
|---|---|
| `GET /health` | `ok` se o Mongo responde |
| `GET /codes` | tabelas fixas do padrão (direção, clima, cor...) para os dropdowns |
| `GET /equipments?q=mf47` | busca equipamentos para o autocomplete (contém, ignora maiúscula) |
| `GET /cameras?q=hik` | busca câmeras para o autocomplete |
| `POST /equipments` | cadastra equipamento `{"name": "8003 MF4739"}` → `8003-MF4739` |
| `POST /cameras` | cadastra câmera `{"name": "hikvision 233"}` → `hikvision-233` |
| `POST /packages` | cria pacote `{"name": "Noturnas Setembro"}` → `Noturnas-Setembro` |
| `GET /packages?q=noturnas` | pacotes (mais novos primeiro) com progresso: `total`, `verified`, `pending`, `in_progress` |
| `POST /packages/{name}/next` | pega a próxima imagem da fila (`X-User` obrigatório; corpo opcional `{"skip": "<sha1>"}`) |
| `POST /images/exists` | `{"sha1s": [...]}` → quais já existem (e em que pacote) e quais faltam |
| `POST /images?package=X` | upload multipart (campo `files`, vários arquivos) |
| `GET /images` | busca com filtros e paginação (veja abaixo) |
| `GET /images/{sha1}` | detalhe de uma imagem (inclui o rascunho salvo) |
| `GET /images/{sha1}/file` | o arquivo da imagem |
| `PUT /images/{sha1}` | salva o rascunho, ou verifica com `"complete": true` |
| `POST /images/{sha1}/release` | devolve a imagem para a fila |

### Paginação

Toda listagem responde no mesmo formato, e aceita `?limit=` e `?skip=`:
```json
{ "total": 1000, "items": [ ... ] }
```

| Rota | `limit` padrão | `limit` máximo |
|---|---|---|
| `GET /images` | 50 | 500 |
| `GET /packages` | 20 | 100 |
| `GET /equipments`, `GET /cameras` | 10 | 50 |

O `total` é o número de resultados **com o filtro aplicado**, para o front montar
"página 3 de 20" ou "e mais 35 resultados".

**Autocomplete:** o front chama `GET /equipments?q=<o que foi digitado>` com debounce
(uns 300 ms depois da última tecla) e mostra os 10 primeiros. A busca é "contém":
`mf47` encontra `8003-MF4739`. O texto é escapado antes de virar regex no Mongo, então
caracteres como `.` e `*` são tratados como texto comum.

**Filtros do `GET /images`:** `package`, `status`, `equipment`, `camera`, `day_time`,
`weather`, `direction`, `vehicle_type`, `vehicle_color`, `plate`, `limit` (padrão 50,
máx. 500) e `skip`. Os filtros de veículo valem para o **mesmo** veículo (`$elemMatch`):
"caminhão branco" não traz uma imagem com um caminhão preto e um carro branco.

**Códigos de resposta mais comuns:**

| Código | Quando |
|---|---|
| `400` | dado inválido; a mensagem diz o quê |
| `404` | imagem ou pacote não existe |
| `409` | duplicado, imagem com outra pessoa, editada no meio, ou rascunho em imagem já verificada |
| `204` | fila vazia (`/next`) |

### Fila de trabalho

- `next` entrega a imagem pendente livre mais antiga e a reserva para o usuário por **15 minutos**.
- Recarregar a página devolve a **mesma** imagem.
- Pular (`skip`) manda a imagem para o fim da fila.
- Salvar rascunho renova o prazo, e verificar tira a imagem da fila.
- Se a pessoa sumir, o prazo vence e a imagem volta para a fila sozinha.
- Outra pessoa não consegue editar uma imagem reservada (`409`).
- A reserva é um `FindOneAndUpdate` atômico: duas pessoas clicando ao mesmo tempo
  nunca recebem a mesma imagem.
- Toda gravação usa **optimistic locking** pelo `updated_at`: se alguém gravou no meio,
  volta `409` e nada é perdido.

---

## Rodando

### 1. Configuração
```powershell
copy .env.example .env
```
Edite o `.env` e troque as senhas. Use só letras e números nelas.

### 2. Mongo
```powershell
docker compose up -d mongo
docker ps        # espere o sidm-mongo ficar (healthy)
```
O `mongo/init/` só roda na **primeira** criação do volume. Para recriar do zero
(apaga todos os dados):
```powershell
docker compose down -v
Remove-Item -Recurse -Force data\images
```

### 3. Backend

**Local** (para desenvolver), em `backend/`:
```powershell
go run ./cmd/server
```
O `go run` lê o `.env` da raiz sozinho e monta a URI do Mongo com o `sidm_app`.
As imagens ficam em `SIDM/data/images`.

> Variáveis setadas no terminal (`$env:HTTP_ADDR=...`) têm prioridade sobre o `.env`.

**Tudo no Docker:**
```powershell
docker compose up -d --build
```
O backend fica em `http://localhost:<BACKEND_PORT>`.

### 4. Teste rápido
```powershell
# cadastros
Invoke-RestMethod -Method Post -Uri http://localhost:8080/equipments -ContentType "application/json" -Body '{"name":"8003 MF4739"}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/cameras    -ContentType "application/json" -Body '{"name":"hikvision 233"}'
Invoke-RestMethod -Method Post -Uri http://localhost:8080/packages   -ContentType "application/json" -Headers @{"X-User"="nathan"} -Body '{"name":"pacote 8003"}'

# upload (curl.exe, não o alias curl do PowerShell)
curl.exe -F "files=@C:\caminho\img1.jpg" -F "files=@C:\caminho\img2.jpg" "http://localhost:8080/images?package=pacote-8003"

# fila e progresso
Invoke-RestMethod -Method Post -Uri http://localhost:8080/packages/pacote-8003/next -Headers @{"X-User"="nathan"}
Invoke-RestMethod http://localhost:8080/packages
```

---

## Testes

Em `backend/`:
```powershell
go test ./... -cover
```

| Pacote | Tipo |
|---|---|
| `filename`, `files`, `cmd/server` | unitários |
| `api` | handlers testados com um banco em memória (`memStore`), sem Docker |
| `store` | **integração** com o Mongo de verdade |

Os testes do `store` usam o usuário root do `.env` e um banco descartável
(`sidm_test_<timestamp>`), apagado no fim. O banco `sidm` nunca é tocado.
Sem Mongo rodando, eles aparecem como `SKIP`.

Para ver a cobertura linha por linha:
```powershell
go test ./... -coverprofile=cover.out
go tool cover -html=cover.out
```

---

## Decisões de projeto

- **SHA-1 do conteúdo = `_id`:** duplicata é impossível por construção. O hash é calculado
  enquanto o arquivo é gravado, sem ler duas vezes nem carregar na memória.
- **Nome autodescritivo + banco:** a pasta faz sentido sem o sistema, e o banco permite
  buscar. O `Format()` mantém os dois sincronizados.
- **Códigos fixos no Go, cadastros no Mongo:** o padrão só muda com revisão e deploy, mas
  um equipamento novo entra sem deploy.
- **Banco e disco nunca divergem:** quando uma operação mexe nos dois e a segunda parte
  falha, a primeira é desfeita.
- **Menor privilégio:** o backend usa o `sidm_app`, que só tem `readWrite` no banco `sidm`.
- **Erros internos não vazam:** o cliente recebe "erro interno", e o detalhe vai para o log.
- **A API depende de uma interface:** os handlers são testados sem banco.

---

## Próximos passos

- [ ] **Frontend** (React): upload de pacote, anotação com a fila, busca
- [ ] **Export** (`POST /export`): zip das imagens filtradas ou de um pacote
- [ ] **Login próprio** (usuário + senha bcrypt + sessão) no lugar do `X-User`
- [ ] Graceful shutdown
- [ ] Validador `$jsonSchema` no Mongo
- [ ] Pré-preenchimento a partir dos nomes originais, quando houver padrão
