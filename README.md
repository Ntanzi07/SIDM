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
                       guarda o INI embutido)                                                               (o arquivo fica <sha1>)
```

- **Pacote:** um lote de imagens enviado junto. Cada pacote é uma pasta no servidor,
  e cada imagem pertence a **um** pacote só.
- **Duplicatas:** o SHA-1 do conteúdo é a identidade da imagem. Uma imagem que já
  existe em outro pacote é **pulada** no upload (`duplicate`, com o `existing_package`).
- **Rascunho:** `PUT` com `"complete": false` salva o que já foi preenchido, sem validar
  tudo e sem renomear. A imagem continua `pending`.
- **Verificar:** `PUT` com `"complete": true` valida tudo pelo perfil de anotação do
  pacote (inclusive os obrigatórios), confere o cadastro de equipamento e câmera,
  calcula o nome v2 (no perfil `veiculos`) e marca `verified`. **O arquivo não muda
  de nome:** no disco toda imagem é `<sha1>.ext`, e o nome de saída é escolhido no
  export (veja [Nomes no disco e no export](#nomes-no-disco-e-no-export)).
- **Perfil de anotação:** cada pacote tem um, e ele diz o que se anota (veja
  [Perfis de anotação](#perfis-de-anotação)). A imagem guarda a anotação em
  `fields` (os campos) e `objects` (os objetos, ex.: os veículos).
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
├── frontend/               React + Vite (veja frontend/PLAN.md)
└── backend/                Go
    ├── cmd/server/         main: lê o .env, conecta no Mongo, monta os módulos, sobe o HTTP
    └── internal/
        ├── platform/       o que todos usam: erros, respostas HTTP, usuário, conexão e Mongo de teste
        ├── catalog/        MÓDULO: pacotes, equipamentos e câmeras (regras, Mongo, HTTP)
        ├── images/         MÓDULO: upload, INI, fila, anotação, verificação (nome v2), histórico
        ├── filename/       padrão v2: Parse, Format, tabelas de códigos, sanitização
        ├── files/          disco: hash durante a gravação, pasta por pacote, rename
        ├── importer/       leitura de CSV e de INI, perfis de import (sem banco, sem HTTP)
        ├── imports/        MÓDULO: perfis de import, raio-x e import de CSV/INI nos rascunhos
        ├── crypt/          abre as imagens criptografadas pelos equipamentos
        ├── boxes/          MÓDULO: tarefas e caixas do CVAT (leitura do COCO, import, cvat.zip)
        ├── export/         datasets de treino: perfis, divisão, pastas, nomes, COCO/YOLO/VOC (sem banco, sem HTTP)
        ├── exports/        MÓDULO: perfis de export salvos, preview e download do zip
        ├── annotation/     MÓDULO: perfis de anotação (campos da imagem e tipos de objeto)
        └── api/            junta as rotas dos módulos: /health, /docs + openapi.yaml
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

### Campo desconhecido = posição vazia

Só o sha1 e a versão são obrigatórios. O que a imagem não tem, ou não dá para
conferir (data, câmera, equipamento, placa ilegível, cor...), fica **vazio** na
anotação e vira uma **posição vazia** no nome. Nenhum valor real é vazio, então
nunca há ambiguidade, e o nome continua sendo lido de volta igual.

| Caso | Trecho do nome |
|---|---|
| sem câmera | `..._8003-MF4739__F-08-...` |
| sem data | `..._01__ABC1D23-M-8-T-WHT.jpg` |
| placa ilegível | `..._20260903T112523Z_-M-8-T-WHT.jpg` |
| sem veículo nenhum | `..._20260903T112523Z_.jpg` |
| sem condições | `..._F-08-L-D-E-S__2026...` |
| só a direção desconhecida | `..._-08-L-D-E-S_...` |
| tudo desconhecido | `<sha1>_v2___-----___.jpg` |

Um veículo com tudo desconhecido é `----` (é diferente de não ter veículo).
Na tela de anotação, todo campo tem a opção **"Não informado"**.

---

## API

A referência completa, com exemplos e o "Try it out", está em **`/docs`** (Swagger UI).
A especificação fica em `backend/internal/api/openapi.yaml` e vai embutida no binário.

### Organização do backend (monólito modular)

O backend é **um binário só**, organizado por **assunto** (módulos), e não por
camada. Os módulos são `catalog`, `images`, `boxes`, `imports`, `exports` e
`annotation`. Ao
lado deles ficam as bibliotecas puras, sem banco e sem HTTP (`filename`, `files`,
`crypt`, `importer`, `export`), e o `api`, que só junta as rotas.

Cada módulo segue o mesmo formato (veja `internal/catalog/`):

| arquivo | o que tem |
|---|---|
| `catalog.go` | os tipos e as **interfaces** do que o módulo precisa dos outros |
| `service.go` | as regras (é o que os outros módulos chamam) |
| `mongo.go` | as coleções **dele** (e de nenhuma outra) |
| `memory.go` | o mesmo repositório em memória, para os testes |
| `http.go` | os handlers e a lista de rotas (`Routes()`) |

**Regras:**
- um módulo só lê e escreve as coleções dele;
- o que ele precisa de outro, pede por uma interface pequena declarada nele
  (ex.: o catálogo declara `Images`, com o progresso dos pacotes e as cascatas);
- as cascatas viram **avisos**: ao renomear ou apagar um pacote, o catálogo avisa
  quem guarda o pacote (`PackageListener`: as caixas); ao apagar uma imagem, o
  `images` avisa as caixas (`Listener`); ao renomear uma tarefa, o `boxes` avisa
  quem guarda a tarefa (`TaskListener`: os perfis de export). Cada um cuida das
  suas coleções;
- o `main.go` monta os módulos e liga as interfaces;
- a API não muda com a reorganização: o frontend não percebe.

| Rota | O que faz |
|---|---|
| `GET /health` | `ok` se o Mongo responde |
| `GET /codes` | tabelas fixas do padrão (direção, clima, cor...) para os dropdowns |
| `GET /equipments?q=mf47` | busca equipamentos para o autocomplete (contém, ignora maiúscula) |
| `POST /equipments` | cadastra equipamento `{"name": "8003 MF4739"}` → `8003-MF4739` |
| `PATCH /equipments/{name}` | renomeia em cascata (`{"name"}`) ou ativa/desativa (`{"active"}`) |
| `DELETE /equipments/{name}` | apaga; se estiver em uso, só desativa |
| `GET /cameras?q=hik` | busca câmeras para o autocomplete |
| `POST /cameras` | cadastra câmera `{"name": "hikvision 233"}` → `hikvision-233` |
| `PATCH /cameras/{name}` | igual ao de equipamentos |
| `DELETE /cameras/{name}` | igual ao de equipamentos |
| `GET /packages?q=noturnas` | pacotes (mais novos primeiro) com progresso: `total`, `verified`, `pending`, `in_progress` |
| `POST /packages` | cria pacote `{"name": "Noturnas Setembro"}` → `Noturnas-Setembro` |
| `PATCH /packages/{name}` | renomeia o pacote `{"name": "..."}` (pasta + caminho das imagens) |
| `DELETE /packages/{name}?confirm={name}` | **apaga tudo**: pacote, imagens, arquivos e histórico |
| `POST /packages/{name}/next` | pega a próxima imagem da fila (`X-User` obrigatório; corpo opcional `{"skip": "<sha1>"}`) |
| `POST /packages/{name}/import` | importa um CSV (ou aplica um perfil de INI, sem arquivo) para os rascunhos; ensaio por padrão |
| `GET /images` | busca com filtros e paginação (veja abaixo) |
| `POST /images?package=X` | upload multipart (campo `files`, vários arquivos); `&profile=` aplica um perfil de INI |
| `POST /images/exists` | `{"sha1s": [...]}` → quais já existem (e em que pacote) e quais faltam |
| `GET /images/{sha1}` | detalhe de uma imagem (inclui o rascunho salvo) |
| `PUT /images/{sha1}` | salva o rascunho, ou verifica com `"complete": true` |
| `DELETE /images/{sha1}` | apaga a imagem: registro, histórico, caixas e arquivo (recusa se outra pessoa está anotando) |
| `GET /images/{sha1}/file` | o arquivo da imagem |
| `GET /images/{sha1}/boxes` | as caixas (CVAT) da imagem, em cada tarefa |
| `POST /images/{sha1}/release` | devolve a imagem para a fila |
| `POST /images/{sha1}/reopen` | reabre para edição: uma verificada volta a pendente e fica reservada para quem pediu |
| `POST /imports/inspect` | "raio-x" de um CSV (ou, com `?type=jpeg-ini`, do INI de até 20 imagens): colunas, exemplos e valores distintos |
| `GET /packages/{name}/cvat.zip` | as imagens do pacote, para criar a task no CVAT |
| `GET /packages/{name}/boxes` | por tarefa: imagens do pacote anotadas nela |
| `GET /tasks` | tarefas de anotação de caixas, com o resumo de cada uma |
| `POST /tasks` | cria uma tarefa (`name`, `description`, `labels`) |
| `GET`/`PUT`/`DELETE /tasks/{name}` | uma tarefa: resumo por pacote e label; renomear leva caixas e perfis junto; apagar leva as caixas |
| `POST /tasks/{name}/import` | importa o export COCO 1.0 do CVAT na tarefa, procurando as imagens em todos os pacotes; ensaio por padrão |
| `GET /export-profiles` | perfis de export: os padrões (`rf-detr`, `yolo`) e os salvos |
| `POST /export-profiles` | cria um perfil de export |
| `GET`/`PUT`/`DELETE /export-profiles/{name}` | um perfil de export (os padrões não mudam) |
| `POST /exports/preview` | prévia do dataset: imagens por grupo, caixas por classe |
| `POST /exports/download` | o zip do dataset |
| `GET /annotation-profiles` | perfis de anotação: o padrão (`veiculos`) e os salvos |
| `POST /annotation-profiles` | cria um perfil de anotação (os erros voltam todos juntos, em `problems`) |
| `GET`/`PUT`/`DELETE /annotation-profiles/{name}` | um perfil de anotação (o padrão não muda) |
| `POST /annotation-profiles/{name}/check` | confere uma anotação contra o perfil, sem gravar: valores limpos ou problemas |
| `GET /export-folder-fields` | metadados que podem virar pasta no VOC por pastas, com o nome padrão de cada valor |
| `GET /import-profiles` | lista os perfis de import |
| `POST /import-profiles` | cria um perfil de import |
| `GET /import-profiles/{name}` | um perfil |
| `PUT /import-profiles/{name}` | atualiza um perfil (manda o JSON inteiro de novo) |
| `PATCH /import-profiles/{name}` | renomeia o perfil `{"name": "..."}` |
| `DELETE /import-profiles/{name}` | apaga o perfil (os rascunhos que ele preencheu continuam) |
| `GET /docs` | documentação interativa (Swagger UI) |
| `GET /openapi.yaml` | a especificação OpenAPI |

**Filtros do `GET /images`:** `package`, `status`, `fields.<campo>` (um campo da
imagem; numa lista, basta conter o valor) e `objects.<campo>`. Os filtros de objeto
valem para o **mesmo** objeto (`$elemMatch`): `objects.type=truck&objects.color=white`
não traz uma imagem com um caminhão preto e um carro branco. Os nomes de antes dos
perfis continuam valendo: `equipment`, `camera`, `day_time`, `weather`, `direction`,
`vehicle_type`, `vehicle_color` e `plate`.

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

### Ver e reabrir imagens verificadas

No detalhe do pacote, **Ver verificadas** abre as imagens já anotadas, só para
leitura (com as caixas do CVAT por cima), navegando com anterior/próxima. Clicar
no nome de uma imagem na lista também abre: as verificadas na visualização, as
pendentes direto na anotação.

**Editar** chama `POST /images/{sha1}/reopen`: a imagem volta para **pendente**
(o nome v2 calculado sai do banco até ela ser verificada de novo), a anotação atual
fica como rascunho, e ela já fica **reservada** para quem clicou, para ninguém pegar
na fila no meio. A reabertura vai para o histórico. As caixas não mudam.

### Editar e apagar cadastros e pacotes

**Equipamentos e câmeras**

- **Renomear** (`PATCH {"name": "..."}`) é em **cascata**:
  - as pendentes trocam o nome no banco, numa operação só;
  - nas verificadas, o nome v2 (no banco) é calculado de novo, uma a uma, com o
    histórico registrando a troca (`changed_by` = `<usuário> (renomear equipment)`).
    Nenhum arquivo muda no disco.
- Se o nome novo **já existe**, os dois são **fundidos** (`"merged": true`). Serve para
  limpar duplicatas que o import criou (`8003-MF4739` e `8003-mf4739`, por exemplo).
- Se alguma verificada falhar (arquivo em uso, banco fora), ela vai em `failed`, o nome
  antigo **fica** e a resposta vem com `"complete": false`. É só repetir o **mesmo**
  pedido: ele continua de onde parou.
- **Desativar** (`PATCH {"active": false}`): some do autocomplete e não pode ser usado em
  novas verificações. As imagens que já usam ficam como estão.
- **Apagar** (`DELETE`): se nenhuma imagem usa, apaga de vez. Se alguma usa, só
  **desativa**, para a busca por aquele equipamento continuar funcionando.

**Pacotes**

- **Renomear** (`PATCH`): renomeia a pasta `dataset/<pacote>/` e atualiza o cadastro e o
  `path` de todas as imagens. O nome dos arquivos não muda (o pacote não está no nome v2).
  Se o banco falhar, a pasta volta ao nome antigo.
- **Apagar** (`DELETE`) é **irreversível**: apaga o pacote, as imagens (banco e arquivos)
  e o histórico. Para não ser por engano, o nome tem que ser repetido em `?confirm=`:
  ```
  DELETE /packages/Noturnas-Setembro?confirm=Noturnas-Setembro
  ```
  O banco é apagado primeiro. Se a pasta não puder ser removida (arquivo aberto no
  Windows, por exemplo), a resposta traz um aviso em `warnings` e a pasta pode ser
  apagada à mão.

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
assinatura. O SIDM nunca recomprime, limpa ou reescreve o arquivo (nem renomeia: no
disco ele é sempre `<sha1>.ext`).
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

## Caixas (bounding boxes) do CVAT, por tarefa

O SIDM **não desenha** caixas: elas são anotadas no **CVAT**, e o SIDM guarda o
resultado para a exportação de treino.

As caixas são organizadas em **tarefas** (tela **Tarefas**): cada projeto de
anotação é uma tarefa, com as suas labels. Por exemplo, `capacete`
(`com_capacete`, `sem_capacete`) e `placa-letras` (`A`…`Z`, `0`…`9`). **A mesma
imagem pode estar em várias tarefas** sem que uma sobrescreva a outra. Os pacotes
de uma tarefa não são escolhidos: são os das imagens anotadas nela.

1. **Crie a tarefa** em **Tarefas → + Nova tarefa** (nome em minúsculas, sem
   espaço; as labels, uma por linha).
2. **Ida:** baixe as imagens para o CVAT (na tarefa, ou no detalhe do pacote:
   `GET /packages/{nome}/cvat.zip`). Elas vão com os nomes guardados no SIDM, que
   começam pelo sha1. Crie a task no CVAT com esse zip (dá para juntar zips de
   vários pacotes numa task só).
3. **No CVAT:** anote e exporte a task no formato **COCO 1.0**.
4. **Volta:** na página da tarefa, em **Importar caixas do CVAT**, suba o zip
   exportado (ou só o `.json`). Primeiro vem o **ensaio**, depois **Gravar caixas**.

**Regras do import:**
- As imagens são procuradas no **banco todo** (qualquer pacote). **Casamento** pelo
  nome do arquivo (sem a pasta): se começa com um sha1, é por ele; senão, pelo
  **nome original**, só se for único no banco todo (nomes como `IMG_0001.jpg` se
  repetem entre câmeras: na dúvida, a imagem fica de fora). Usar o zip do passo 2
  evita qualquer ambiguidade.
- Imagens **sem caixa** no arquivo: por padrão **não são mexidas**, porque o CVAT
  exporta todas as imagens da task, inclusive as que ninguém anotou ainda. Marque
  **"Imagens sem nenhuma caixa entram como sem objetos"** só quando a task do CVAT
  estiver toda anotada: aí elas entram na tarefa como **negativas** (sem objetos),
  e o export as inclui sem caixas.
- Imagens com **tamanho diferente** no CVAT e no SIDM são **puladas** (as caixas
  ficariam no lugar errado).
- **Reimportar substitui** as caixas da imagem **naquela tarefa** (as outras
  tarefas não mudam).
- Labels que a tarefa ainda não tem são **acrescentadas** à lista dela.
- Caixas com problema (categoria desconhecida, largura zero) são ignoradas e
  aparecem nos avisos do ensaio.

**Como fica no banco** (coleção `image_boxes`, um documento por tarefa e imagem;
as tarefas ficam em `annotation_tasks`):

```json
{
  "task": "veiculos", "image": "a3f9...", "package": "Noturnas-Setembro",
  "width": 1920, "height": 1080,
  "boxes": [
    { "label": "carro", "x": 410.5, "y": 220, "w": 820, "h": 610 },
    { "label": "placa", "x": 790, "y": 640, "w": 140, "h": 45, "occluded": true,
      "attributes": { "caractere": "A" } }
  ],
  "source": { "tool": "cvat", "format": "coco", "file": "task_12.zip", "file_name": "a3f9....jpg" },
  "imported_by": "nathan", "imported_at": "2026-11-17T11:40:00Z"
}
```

As caixas ficam em **pixels** (`x, y` = canto superior esquerdo). `occluded` e
`rotation` vêm do CVAT; outros atributos criados lá ficam em `attributes`.
`"boxes": []` é uma imagem negativa. Renomear ou apagar o pacote (ou a imagem) leva
as caixas junto; renomear a tarefa leva as caixas e os perfis de export que apontam
para ela; apagar a tarefa apaga só as caixas dela (as imagens ficam). Na tela de
anotação e no visualizador, as caixas de todas as tarefas aparecem por cima da
imagem, só para conferência.

**Caixas de antes das tarefas:** ao subir, o backend move as caixas antigas (um
conjunto por imagem) para a tarefa **`padrao`**, criada com as labels delas. Dá para
renomeá-la depois.

---

## Perfis de anotação

Um perfil diz **o que se anota** em cada imagem:

- **campos da imagem** (ex.: direção, clima, dia/noite);
- **tipos de objeto**, cada um com os seus campos. Um tipo pode ter **caixa**
  (é uma label do CVAT, e cada caixa ganha os campos, ex.: `capacete` com `cor`)
  ou não (é uma lista na imagem, como os veículos com placa).

| tipo de campo | o que guarda | opções do campo |
|---|---|---|
| `choice` | um valor da lista | `options` (`value`, `label`, `code`) |
| `multi` | vários valores da lista | `options` |
| `text` | texto livre | `pattern` (o valor inteiro), `max_len`, `upper` |
| `number` | número | `min`, `max`, `integer` |
| `bool` | sim/não | |
| `datetime` | data e hora (guardada em UTC) | |
| `ref` | um nome do cadastro | `ref`: `equipment` ou `camera` |

O `code` de uma opção é a forma curta que vai no nome do arquivo (ex.: `night` → `N`).
Se uma opção do campo tem código, todas precisam ter.

O padrão **`veiculos`** é o que o SIDM sempre anotou, com os mesmos códigos do nome
v2 (as opções vêm das mesmas tabelas, então não saem de sincronia). Ele não pode ser
alterado: para mudar, salve uma cópia com outro nome.

```json
{
  "name": "capacete-moto",
  "fields": [
    {"key": "day_time", "label": "Dia/noite", "type": "choice", "required": true,
     "options": [{"value": "day", "label": "Dia", "code": "D"}, {"value": "night", "label": "Noite", "code": "N"}]},
    {"key": "riders", "label": "Pessoas na moto", "type": "number", "integer": true, "min": 1, "max": 3}
  ],
  "objects": [
    {"key": "helmet", "label": "Capacete", "box": true,
     "fields": [{"key": "color", "type": "choice", "options": [{"value": "black"}, {"value": "white"}]}]}
  ]
}
```

Na tela, os perfis ficam em **Configurar → Perfis de anotação**: criar, editar, duplicar
(o `veiculos` só pode ser duplicado) e apagar. Os erros do perfil voltam todos juntos
e a linha de cada campo com problema fica marcada. Um perfil **em uso** por algum
pacote não pode ser apagado: troque o perfil desses pacotes antes.

Para testar um perfil antes de usar, o `POST /annotation-profiles/{name}/check` confere
uma anotação sem gravar nada.

Um campo pode ter **valor inicial** (`default`): é o que a tela já traz numa imagem
sem rascunho (no `veiculos`: traseira, diurno, sem chuva...).

### O perfil no pacote e nas imagens

- **Cada pacote escolhe um perfil** ao ser criado (`POST /packages` com `"profile"`;
  sem ele, `veiculos`). Dá para trocar (`PATCH /packages/{nome}`) **só enquanto o
  pacote não tem imagem verificada**: o nome do arquivo delas saiu do perfil atual.
- **A imagem guarda a anotação** em `fields` e `objects`, e o `PUT` passa tudo pelo
  `Check` do perfil. Os erros voltam juntos, em `problems`, e a tela marca cada campo.

```json
PUT /images/{sha1}
{
  "fields": {"direction": "rear", "day_time": "night", "conditions": ["glare"]},
  "objects": [{"type": "vehicle", "fields": {"plate": "ABC1D23", "type": "car"}}],
  "complete": true
}
```

- **O nome:** no disco, sempre `<sha1>.ext`. No `veiculos`, a verificada também
  guarda o nome v2 no banco (`file_name`), que o export usa.
- **Import e INI** só preenchem o que o perfil do pacote tem; o resto aparece no
  relatório, em `outside_profile`. Os perfis de import (e os filtros e pastas do
  export) ainda escrevem e leem os campos do `veiculos`.
- **Migração:** na subida, o servidor leva as imagens de antes dos perfis (campos
  soltos e `vehicles`) para `fields`/`objects`, numa operação no próprio Mongo. Só
  roda uma vez: nas próximas subidas não há nada para migrar.

---

## Nomes no disco e no export

**No disco, toda imagem é `dataset/<pacote>/<sha1>.ext`**, pendente ou verificada.
Verificar, reabrir ou renomear um equipamento nunca move arquivo. Assim:

- o caminho de uma imagem nunca muda (só se o pacote for renomeado);
- nenhum perfil de anotação precisa saber montar nome de arquivo;
- o nome com os metadados é escolhido **na saída**, no perfil de export.

No perfil `veiculos`, a verificada guarda também o **nome v2** no banco (`file_name`,
com `pattern_version: "v2"`). É o nome que o modo "Nome do SIDM (v2)" do export usa;
nos outros perfis esse modo usa o `<sha1>`. Os modelos de nome do export (o padrão
antigo, ou um personalizado) montam o nome a partir dos campos.

As verificadas de antes desta regra (com o nome v2 no arquivo) voltam para
`<sha1>.ext` na primeira subida do servidor, uma a uma. Se alguma falhar, o log avisa
e ela é tentada de novo na próxima subida.

---

## Export para treino (RF-DETR, YOLO, Pascal VOC)

Na tela **Exportar**: escolha um perfil e a **tarefa**, veja a **prévia** e baixe
o zip.

**O que entra:** imagens **verificadas** e **anotadas na tarefa** escolhida que
passam nos filtros do perfil. As anotadas como "sem objetos" entram sem caixas
(negativas). Imagens pendentes não entram. Os perfis padrão servem para qualquer
tarefa; um perfil próprio guarda a tarefa junto.

**Perfis padrão** (não podem ser alterados; para mudar algo, salve com outro nome):

| Perfil | Formato | Divisão |
|---|---|---|
| `rf-detr` | COCO no layout do Roboflow: `train/`, `valid/`, `test/`, cada um com as imagens e `_annotations.coco.json` | 70/15/15 |
| `yolo` | Ultralytics: `images/{train,val,test}`, `labels/{train,val,test}` e `data.yaml` | 70/15/15 |
| `voc` | Pascal VOC: `train/`, `valid/`, `test/`, cada imagem com o seu `.xml` ao lado | 70/15/15 |
| `voc-pastas` | Pascal VOC em pastas pelos metadados: `Car/01-640x480/Br/Traseiro/Noturno/` (tipo de veículo / resolução / tipo de placa / direção / período), com o nome no **padrão antigo** | sem divisão |

**Um perfil próprio controla:**
- **tarefa** (as caixas de qual tarefa entram);
- **formato e divisão** (porcentagens que somam 100; no YOLO a validação não pode ser 0);
- **nome dos arquivos**: o do SIDM (v2), o **padrão antigo** (hash +
  características + placa, ex.: `02d9…3a3fF01LDBEB1S01KOY7386.jpg`) ou um modelo
  próprio com campos entre chaves, como `placa-{plate}_{sha1}`. Os campos de código
  (`{direction}`, `{resolution}`, `{capture_angle}`, `{day_time}`, `{vehicle_type}`,
  `{image_range}`, `{plate_type}`, `{plate_category}`, `{weather}`, `{condition}`,
  `{vehicle_color}`) saem com o código do nome v2 (F/T, 01…10, …); há também
  `{sha1}`, `{plate}`, `{captured_at}`, `{date}`, `{name}`, `{original}`,
  `{package}`, `{equipment}` e `{camera}`. Campo sem valor vira `_` na largura dele
  (o padrão antigo continua com tamanho fixo); os do veículo vêm do primeiro
  veículo e a condição é a primeira da imagem. O modelo tem que ter `{sha1}` (ou
  `{name}`), para os nomes não repetirem. O `.xml`, o `.txt` e o COCO usam o nome
  novo;
- **pastas de saída** (só no VOC): **por divisão** (`train/valid/test`) ou **por
  pastas**, uma árvore com os níveis que você escolhe, na ordem que quiser: tipo de
  veículo, tipo e categoria da placa, cor, resolução, direção, período, clima,
  ângulo, alcance, equipamento, câmera, pacote e a própria divisão. Cada valor
  ganha o nome de pasta que você der (`rear` → `Traseiro`); sem nome, vale o padrão
  (o próprio valor; na resolução, `01-640x480`). Imagens sem o dado vão para
  `Desconhecido` (ou o nome escolhido). Os dados do veículo vêm do primeiro veículo
  da imagem. Sem o nível "Divisão", a divisão não é usada;
- **pacotes** (vazio = todos);
- **filtros de metadados** (período, clima, direção, tipo e cor do veículo, equipamento, câmera);
- **labels**: quais entram e com que nome. Dar o mesmo nome a duas labels junta as
  classes (`Placa` e `placa` viram `placa`). A ordem das classes define os ids
  (YOLO começa em 0, COCO em 1) e aparece na prévia.

**A divisão é fixa por imagem:** o grupo (train/valid/test) de cada imagem vem do
sha1. Re-exportar com imagens novas não move uma imagem do test para o train, então
dá para comparar treinos.

**No zip:**
- `export_info.json` com o perfil usado, a data, quem exportou e as contagens;
- `FALTANDO.txt`, se alguma imagem não pôde ser lida do disco;
- caixas **rotacionadas** do CVAT viram o menor retângulo reto que as contém (COCO e
  YOLO de detecção só têm caixa reta);
- o YOLO sai pronto para `yolo train data=data.yaml` depois de descompactar;
- no VOC, cada imagem tem o seu `.xml` ao lado (coordenadas em pixels inteiros:
  `xmin, ymin, xmax, ymax`; `truncated` = caixa encostada na borda; `occluded` vem
  do CVAT), e um `labels.txt` na raiz lista as classes.

O zip é gerado enquanto baixa: não ocupa disco no servidor, e o navegador salva
direto no disco.

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
| `filename`, `files`, `crypt`, `importer`, `export`, `cmd/server` | unitários |
| `api` | o servidor inteiro por HTTP, com os módulos usando os repositórios em memória (`memory.go`), sem Docker |
| `mongo_test.go` de cada módulo | **integração** com o Mongo de verdade |

O `-cover` só conta o que os testes **do próprio pacote** executam. Como a maior
parte dos módulos é testada pela API, para ver quanto de um módulo está coberto:
```powershell
go test ./internal/api -coverpkg=./internal/images,./internal/exports
```

Os testes de integração usam o usuário root do `.env` e um banco descartável
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
  não mexe nos bytes. No disco o nome é sempre `<sha1>.ext`: o nome com os metadados
  (v2) fica no banco e só aparece no export. O INI vai para o banco como uma **lista** de `{section, key, value}`, e
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

- [ ] **Login próprio** (usuário + senha bcrypt + sessão) no lugar do `X-User`
- [ ] Embutir o Swagger UI no binário (hoje vem de um CDN; não abre em rede sem internet)
- [ ] Graceful shutdown
- [ ] Validador `$jsonSchema` no Mongo
- [ ] Regra para veículo com placa ilegível
- [ ] Detectar o perfil de INI sozinho, pelo modelo do equipamento (`Metrologico.Modelo`)
- [ ] Caixa da placa que vem no INI (`OCRPosicaoLeitura`) como caixa pré-anotada
- [ ] Leitor de EXIF para o import
