# SIDM

Sistema de controle e anotação de imagens de veículos.

As imagens chegam em **pacotes**, sem padrão nenhum no nome. O time anota cada
imagem (equipamento, câmera, ângulo, clima, veículos, placas...), confere e
**verifica**. A imagem verificada ganha um nome autodescritivo no padrão v2, e
os metadados ficam no MongoDB, pesquisáveis por qualquer campo.

Metadados que já existem em outro lugar podem pré-preencher as anotações:
um **CSV** exportado de outro sistema, ou o **INI que o equipamento grava dentro
do próprio JPEG**.

> **v1 interna:** não há login ainda. O usuário é informado no header `X-User`.
> Rode apenas na rede interna.

**Documentação interativa da API:** com o servidor rodando, abra `http://localhost:8080/docs`.

---

## Como funciona

```
 1. criar pacote         2. upload                3. (opcional) import de CSV         4. fila de anotação          5. verificar
 POST /packages  ──▶  POST /images?package=X ──▶  POST /packages/X/import  ──▶  POST /packages/X/next  ──▶  PUT /images/{sha1}
                      (pending, nome = hash;     (pré-preenche os rascunhos)    (cada pessoa pega uma)      {"complete": true}
                       guarda o INI embutido)                                                               (renomeia para v2)
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
- **Import:** um CSV ou o INI embutido na imagem preenchem os rascunhos automaticamente,
  mas **nunca verificam**. Uma pessoa continua conferindo pela fila
  (veja [Import de CSV](#import-de-csv) e [INI embutido no JPEG](#ini-embutido-no-jpeg)).
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
        ├── importer/       leitura de CSV e de INI, perfis de import (sem banco, sem HTTP)
        ├── crypt/          abre as imagens criptografadas pelos equipamentos
        ├── store/          Mongo: imagens, pacotes, fila, cadastros, perfis, histórico
        └── api/            handlers HTTP (dependem da interface Store) + openapi.yaml
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

A referência completa, com exemplos e o "Try it out", está em **`/docs`** (Swagger UI).
A especificação fica em `backend/internal/api/openapi.yaml` e vai embutida no binário.

| Rota | O que faz |
|---|---|
| `GET /health` | `ok` se o Mongo responde |
| `GET /codes` | tabelas fixas do padrão (direção, clima, cor...) para os dropdowns |
| `GET /equipments?q=mf47` | busca equipamentos para o autocomplete (contém, ignora maiúscula) |
| `POST /equipments` | cadastra equipamento `{"name": "8003 MF4739"}` → `8003-MF4739` |
| `GET /cameras?q=hik` | busca câmeras para o autocomplete |
| `POST /cameras` | cadastra câmera `{"name": "hikvision 233"}` → `hikvision-233` |
| `GET /packages?q=noturnas` | pacotes (mais novos primeiro) com progresso: `total`, `verified`, `pending`, `in_progress` |
| `POST /packages` | cria pacote `{"name": "Noturnas Setembro"}` → `Noturnas-Setembro` |
| `POST /packages/{name}/next` | pega a próxima imagem da fila (`X-User` obrigatório; corpo opcional `{"skip": "<sha1>"}`) |
| `POST /packages/{name}/import` | importa um CSV (ou aplica um perfil de INI, sem arquivo) para os rascunhos; ensaio por padrão |
| `GET /images` | busca com filtros e paginação (veja abaixo) |
| `POST /images?package=X` | upload multipart (campo `files`, vários arquivos); `&profile=` aplica um perfil de INI |
| `POST /images/exists` | `{"sha1s": [...]}` → quais já existem (e em que pacote) e quais faltam |
| `GET /images/{sha1}` | detalhe de uma imagem (inclui o rascunho salvo) |
| `PUT /images/{sha1}` | salva o rascunho, ou verifica com `"complete": true` |
| `GET /images/{sha1}/file` | o arquivo da imagem |
| `POST /images/{sha1}/release` | devolve a imagem para a fila |
| `POST /imports/inspect` | "raio-x" de um CSV: colunas, exemplos e valores distintos |
| `GET /import-profiles` | lista os perfis de import |
| `POST /import-profiles` | cria um perfil de import |
| `GET /import-profiles/{name}` | um perfil |
| `PUT /import-profiles/{name}` | atualiza um perfil |
| `GET /docs` | documentação interativa (Swagger UI) |
| `GET /openapi.yaml` | a especificação OpenAPI |

**Filtros do `GET /images`:** `package`, `status`, `equipment`, `camera`, `day_time`,
`weather`, `direction`, `vehicle_type`, `vehicle_color` e `plate`. Os filtros de veículo
valem para o **mesmo** veículo (`$elemMatch`): "caminhão branco" não traz uma imagem com
um caminhão preto e um carro branco.

**Códigos de resposta mais comuns:**

| Código | Quando |
|---|---|
| `400` | dado inválido; a mensagem diz o quê |
| `404` | imagem, pacote ou perfil não existe |
| `409` | duplicado, imagem com outra pessoa, editada no meio, ou rascunho em imagem já verificada |
| `413` | arquivo de import maior que 20 MB |
| `204` | fila vazia (`/next`) |

Todo erro responde no formato `{"error": "mensagem"}`.

### Paginação

Toda listagem responde no mesmo formato, e aceita `?limit=` e `?skip=`:
```json
{ "total": 1000, "items": [ ... ] }
```

| Rota | `limit` padrão | `limit` máximo |
|---|---|---|
| `GET /images` | 50 | 500 |
| `GET /packages`, `GET /import-profiles` | 20 | 100 |
| `GET /equipments`, `GET /cameras` | 10 | 50 |

O `total` é o número de resultados **com o filtro aplicado**, para o front montar
"página 3 de 20" ou "e mais 35 resultados".

**Autocomplete:** o front chama `GET /equipments?q=<o que foi digitado>` com debounce
(uns 300 ms depois da última tecla) e mostra os 10 primeiros. A busca é "contém":
`mf47` encontra `8003-MF4739`. O texto é escapado antes de virar regex no Mongo, então
caracteres como `.` e `*` são tratados como texto comum.

### Fila de trabalho

- `next` entrega a imagem pendente livre mais antiga e a reserva para o usuário por **15 minutos**.
- Recarregar a página devolve a **mesma** imagem.
- Pular (`skip`) manda a imagem para o fim da fila. A imagem pulada nunca volta na hora,
  a não ser que seja a única pendente que sobrou.
- Salvar rascunho renova o prazo, e verificar tira a imagem da fila.
- Se a pessoa sumir, o prazo vence e a imagem volta para a fila sozinha.
- Outra pessoa não consegue editar uma imagem reservada (`409`).
- A reserva é um `FindOneAndUpdate` atômico: duas pessoas clicando ao mesmo tempo
  nunca recebem a mesma imagem.
- Toda gravação usa **optimistic locking** pelo `updated_at`: se alguém gravou no meio,
  volta `409` e nada é perdido.

---

## Import de CSV

Um CSV novo **não exige código novo**: exige um **perfil**. O perfil é um JSON que diz
como ler o arquivo, qual coluna identifica a imagem e como traduzir cada valor.

```
1. INSPECIONAR    POST /imports/inspect?delimiter=semicolon          → colunas, exemplos, valores distintos
2. CRIAR PERFIL   POST /import-profiles                              → validado antes de salvar
3. ENSAIAR        POST /packages/X/import?profile=Y                  → relatório; NADA é gravado
4. AJUSTAR        PUT  /import-profiles/Y                            → ex: adicionar "Garoa" no map
5. APLICAR        POST /packages/X/import?profile=Y&dry_run=false    → preenche os rascunhos
```

O perfil é criado **uma vez por formato** de CSV. Depois, todo CSV daquele formato é
importado direto, só escolhendo o perfil.

### Exemplo de perfil

```json
{
  "name": "csv-pg-veiculos",
  "reader": { "type": "csv", "delimiter": ";" },
  "match": { "column": "imagem_nome", "by": "original_name" },
  "fields": {
    "equipment":   { "column": "dados_imagem_num_serie" },
    "captured_at": { "column": "dados_imagem_data_infracao",
                     "date_format": "2006-01-02 15:04:05.000",
                     "timezone": "America/Sao_Paulo" }
  },
  "vehicles": { "mode": "row", "fields": {
    "plate": { "column": "Placa Digitada" },
    "type":  { "column": "dados_imagem_classificacao",
               "map": { "AUTOMOVEL": "car", "CAMINHAO": "truck", "MOTOCICLETA": "motorcycle", "ONIBUS": "bus" } }
  }}
}
```

| Parte | O que faz |
|---|---|
| `reader` | `type: "csv"`, com `delimiter` (padrão `,`) e `encoding` (`utf-8` ou `latin1`; o BOM do Excel é removido). Ou `type: "jpeg-ini"` (veja [INI embutido no JPEG](#ini-embutido-no-jpeg)). |
| `match` | só no CSV: a coluna que identifica a imagem, comparada com o nome original (`original_name`) ou com o hash (`sha1`) |
| `fields` | campos da imagem: `equipment`, `camera`, `direction`, `resolution`, `capture_angle`, `day_time`, `image_range`, `weather`, `conditions`, `captured_at` |
| `vehicles` | `mode: "single"` (um veículo por imagem), `"row"` (uma linha por veículo, agrupadas pela imagem) ou `"columns"` (`Placa{n}` na mesma linha, com `max`) |

Cada campo usa **um** de:
- `column`: o valor de uma coluna (no INI, `"Secao.Chave"`);
- `value`: um valor fixo para o arquivo todo;
- `template`: junta várias colunas, como `"{Metrologico.Data} {Metrologico.Hora}"`. Se alguma
  coluna citada estiver vazia, o campo fica vazio, em vez de um valor pela metade.

E pode ter ainda:
- `map`: traduz o valor de origem para o nosso (ignora maiúscula);
- `split`: separa uma lista numa célula só (só em `conditions`);
- `date_format` + `timezone`: o formato da data no layout do Go, e o fuso em que ela foi escrita (só em `captured_at`).

### Regras do import

- **O padrão é ensaio:** só grava com `dry_run=false`.
- **Nunca verifica:** só preenche o rascunho. A imagem continua `pending`.
- **Respeita quem já anotou:** só preenche campos vazios. Para substituir, use `overwrite=true`.
- **Pula** imagens verificadas, imagens que alguém está anotando agora, e nomes originais
  repetidos no pacote (não dá para saber qual é qual).
- **Cadastra equipamentos e câmeras novos** automaticamente ao aplicar, com o nome sanitizado.
  No ensaio, eles só aparecem listados em `new_equipments` / `new_cameras`.
  Um nome impossível de sanitizar (ex: `8003/MF`) não é cadastrado e aparece em `invalid_names`.
- **O histórico registra a origem:** `nathan via import:csv-pg-veiculos`.
- Valor sem tradução **não quebra nada**: o campo fica vazio e o valor aparece em `unmapped`
  no relatório, com a contagem. É a lista do que falta adicionar no perfil.

> **Separador na URL:** use o nome (`semicolon`, `tab`, `pipe`, `comma`). Um `;` cru na
> query string é descartado pelo Go.

---

## INI embutido no JPEG

Equipamentos de fiscalização gravam um **INI depois do fim do JPEG**. Visualizadores
ignoram o que vem depois do fim da imagem, então o arquivo abre normal e o texto fica
"escondido" no mesmo arquivo:

```
FF D8 ... (imagem) ... FF D9
[Metrologico]
Data=18/09/2026
Hora=10:32:28
...
[NMetrologico]
Equip = MFR1-0001
Placa = GGB5H24
```

| Momento | O que acontece |
|---|---|
| **Todo upload** | se o JPEG tem INI, ele é **guardado no banco** (campo `ini` da imagem), na ordem original. Não precisa de perfil. |
| **Upload com `?profile=`** | além de guardar, um perfil do tipo `jpeg-ini` **pré-preenche o rascunho**. Equipamento e câmera novos são cadastrados sozinhos. |
| **Import com perfil de INI** | `POST /packages/X/import?profile=Y` **sem arquivo**: aplica o perfil no INI já guardado das imagens pendentes. Serve para um perfil criado ou corrigido depois do upload. Ensaio por padrão, como no CSV. |

**O arquivo nunca é alterado.** O INI desses equipamentos costuma ser **assinado
digitalmente** (campos `Assinatura` e `Hash`), e qualquer mudança nos bytes invalida a
assinatura. O SIDM só renomeia o arquivo: nunca recomprime, limpa ou reescreve.
A `Assinatura` também **não** vai para o banco, porque só faz sentido junto do arquivo original.

### Exemplo de perfil de INI

```json
{
  "name": "ini-mfr1",
  "reader": { "type": "jpeg-ini" },
  "fields": {
    "equipment":   { "column": "NMetrologico.Equip" },
    "captured_at": { "template": "{Metrologico.Data} {Metrologico.Hora}.{Metrologico.Milisegundo}",
                     "date_format": "02/01/2006 15:04:05.000",
                     "timezone": "America/Sao_Paulo" }
  },
  "vehicles": { "mode": "single", "fields": {
    "plate": { "column": "NMetrologico.Placa" },
    "type":  { "column": "NMetrologico.TipoVeiculo", "map": { "AUTOMOVEL": "car" } }
  }}
}
```

- As chaves são `"Secao.Chave"`. Uma chave que aparece em duas seções (como `Hora`)
  não colide: vira `Metrologico.Hora` e `NMetrologico.Hora`.
- O leitor limpa os espaços em volta do `=` (`Placa = GGB5H24`) e os que sobram nos
  valores (`AUTOMOVEL           `). INI gravado em Latin-1 também funciona.
- O INI não tem `match`: a imagem é a própria origem dos dados.
- Para descobrir as chaves de um equipamento novo, suba uma foto e abra o
  `GET /images/{sha1}`: o campo `ini` lista tudo.

**Avisos no upload:** um valor sem tradução no perfil, ou uma data num formato
inesperado, **não** impede o upload. O resultado da imagem traz um `warnings`
explicando o problema, e o campo fica vazio.

---

## Imagens criptografadas

O upload **detecta e abre sozinho** os arquivos criptografados pelos equipamentos:

| O arquivo... | O SIDM faz |
|---|---|
| começa com `FF D8` (JPEG aberto) | segue normal |
| vira JPEG com o **RC4 legado** | descriptografa e segue com o arquivo aberto (`"decrypted": "rc4-legacy"`) |
| nenhum dos casos | segue as regras de antes (só a extensão importa) |

- O **RC4 legado** usa uma chave fixa (o SHA-256 de um byte zero), igual ao pacote `crypto`
  do equipamento. Não precisa de segredo. O arquivo criptografado pode ter **qualquer
  extensão**: depois de aberto, ele vira `.jpg`.
- O **hash, o INI e o arquivo guardado** são os do original **aberto**. A mesma foto,
  criptografada ou não, é a mesma imagem (duplicata). O que fica no disco é o JPEG+INI
  original, byte a byte, então a assinatura continua válida.
- O campo `encryption` da imagem registra que ela chegou criptografada.
- **No front:** o `POST /images/exists` compara o hash do arquivo que o navegador tem.
  Para um arquivo criptografado, esse hash é o da versão fechada, então ele não detecta a
  duplicata antes do upload. O servidor detecta depois, de qualquer jeito.

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

Se a porta 8080 estiver ocupada, coloque no `.env`:
```env
HTTP_ADDR=:8081
```
Isso só vale para o `go run`: no Docker, o `docker-compose.yml` fixa a porta interna,
e a porta exposta vem do `BACKEND_PORT`.

> Variáveis setadas no terminal (`$env:HTTP_ADDR=...`) têm prioridade sobre o `.env`,
> e valem até a janela do PowerShell ser fechada.
>
> Se a 8080 estiver ocupada pelo próprio `sidm-backend` (aparece no `docker ps`),
> pare só ele com `docker compose stop backend`.

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

Ou use o **`/docs`**: dá para fazer tudo isso pelo navegador, inclusive o upload.

---

## Testes

Em `backend/`:
```powershell
go test ./... -cover
```

| Pacote | Tipo |
|---|---|
| `filename`, `files`, `importer`, `cmd/server` | unitários |
| `api` | handlers testados com um banco em memória (`memStore`), sem Docker |
| `store` | **integração** com o Mongo de verdade |

Os testes do `store` usam o usuário root do `.env` e um banco descartável
(`sidm_test_<timestamp>`), apagado no fim. O banco `sidm` nunca é tocado.
Sem Mongo rodando, eles aparecem como `SKIP`. Entre eles está um teste de concorrência:
20 pessoas pedindo "próxima" ao mesmo tempo, sem nenhuma imagem repetida.

Os testes do INI usam um JPEG **sintético**, montado no próprio teste: nenhuma foto real
(com placa) fica no repositório.

O `TestTodaRotaEstaDocumentada` falha se uma rota existir no código e não no
`openapi.yaml` (ou o contrário). **Toda rota nova precisa entrar na documentação.**

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
- **Perfis de import como dados:** um formato de CSV novo é configuração, não código.
  O perfil é guardado no Mongo como o próprio texto JSON, para qualquer valor do CSV
  (com `.` ou `$`) funcionar como chave do `map`.
- **Criptografia aberta na entrada:** o arquivo criptografado é aberto no upload e guardado
  aberto. Assim o hash, a detecção de duplicata e a leitura do INI funcionam igual para
  todos os arquivos.
- **O arquivo original é intocável:** o INI dos equipamentos é assinado, então o SIDM
  só renomeia. O INI vai para o banco como uma **lista** de `{section, key, value}`, e
  não como mapa, para manter a ordem e para chaves com `.` ou `$` não serem problema.
- **Banco e disco nunca divergem:** quando uma operação mexe nos dois e a segunda parte
  falha, a primeira é desfeita.
- **Menor privilégio:** o backend usa o `sidm_app`, que só tem `readWrite` no banco `sidm`.
- **Erros internos não vazam:** o cliente recebe "erro interno", e o detalhe vai para o log.
- **A API depende de uma interface:** os handlers são testados sem banco.
- **Documentação escrita à mão e conferida por teste:** o `openapi.yaml` é um arquivo só,
  fácil de revisar, e o teste impede que ele fique para trás.

---

## Próximos passos

- [ ] **Frontend** (React): pacotes, upload, anotação com a fila, busca, import
- [ ] **Export** (`POST /export`): zip das imagens filtradas ou de um pacote
- [ ] **Login próprio** (usuário + senha bcrypt + sessão) no lugar do `X-User`
- [ ] Embutir o Swagger UI no binário (hoje vem de um CDN; não abre em rede sem internet)
- [ ] Graceful shutdown
- [ ] Validador `$jsonSchema` no Mongo
- [ ] Regra para veículo com placa ilegível
- [ ] Detectar o perfil de INI sozinho, pelo modelo do equipamento (`Metrologico.Modelo`)
- [ ] `/imports/inspect` aceitar um JPEG e mostrar as chaves do INI
- [ ] Caixa da placa (`OCRPosicaoLeitura`) e marcações próprias, com export para o RF-DETR (COCO, 70/15/15)
- [ ] Leitor de EXIF para o import
