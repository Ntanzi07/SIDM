# Dados

## Disco

```
<IMAGES_DIR>/dataset/<pacote>/<sha1>.ext     toda imagem, pendente ou verificada
```

O caminho só muda se o pacote for renomeado. O nome com metadados (v2) é calculado e
guardado no banco; o export escolhe o nome de saída.

## Coleções

| Coleção | `_id` / chave | Dono |
|---|---|---|
| `images` | `_id` = SHA-1 do conteúdo | `images` |
| `image_history` | uma entrada por campo alterado | `images` |
| `packages`, `equipments`, `cameras` | `name` único | `catalog` |
| `annotation_profiles` | `name` único (o padrão `veiculos` fica no código) | `annotation` |
| `annotation_tasks` | `name` único (a `padrao` existe sempre) | `boxes` |
| `image_boxes` | único por (tarefa, imagem) | `boxes` |
| `import_profiles` | `name` único (o padrão `splice-ini` fica no código) | `imports` |
| `export_profiles` | `name` único (os padrões ficam no código) | `exports` |
| `datasets` | `name` único | `versions` |
| `versions` | `_id` aleatório; único por (`dataset`, `number`) | `versions` |
| `version_items` | único por (`version`, `sha1`): a anotação congelada de cada imagem | `versions` |

Índices: `mongo/init/01-init.js` (banco novo) e os `EnsureIndexes` de cada módulo (toda subida).

## O documento da imagem (resumo)

```json
{
  "_id": "<sha1>", "package": "Noturnas-Setembro", "status": "pending|verified",
  "original_name": "IMG_001.jpg", "ext": ".jpg", "size": 123456,
  "path": "dataset/Noturnas-Setembro/<sha1>.jpg",
  "file_name": "<sha1>_v2_8003-MF4739_..._WHT.jpg",   // só verificada; o nome v2 é um DADO
  "pattern_version": "v2",                             // vazio fora do perfil veiculos
  "fields":  { "equipment": "8003-MF4739", "direction": "rear", "conditions": ["glare"],
               "captured_at": "2026-09-03T11:25:23Z" },
  "objects": [ { "type": "vehicle", "fields": { "plate": "ABC1D23", "type": "car" } } ],
  "ini": [ { "section": "...", "key": "...", "value": "..." } ],   // sem a Assinatura
  "claimed_by": "nathan", "claimed_until": "...",                   // reserva da fila (15 min)
  "updated_at": "..."                                               // optimistic locking
}
```

- **`fields` e `objects` seguem o perfil de anotação do pacote** e passam pelo
  `Profile.Check` ao salvar. Tipos guardados: texto, número (`float64`), sim/não, lista de
  textos (`[]string`), data (`time.Time`, UTC). O `images.MongoRepo` converte o que o Mongo
  devolve (`bson.A`, `bson.DateTime`, `int32`) para esses tipos.
- **Objetos sem caixa** ficam em `objects`. **Objetos com caixa** são as caixas do CVAT,
  em `image_boxes`: a label é a chave do tipo, e `boxes[].attributes` segue os campos
  dele (conferidos no import e na edição; label sem tipo guarda os atributos crus). O
  documento ganha `updated_by`/`updated_at` quando alguém edita os atributos no SIDM
  (o reimport zera; é a versão para o optimistic locking, senão `imported_at`).
- Os campos `ref` têm nome fixo: `equipment` e `camera` (a cascata de renomear procura
  `fields.equipment`/`fields.camera` em qualquer perfil).

## Perfis de anotação

Campos: `choice`, `multi`, `text` (`pattern` do valor inteiro, `max_len`, `upper`),
`number` (`min`, `max`, `integer`), `bool`, `datetime`, `ref` (`equipment`/`camera`).
Cada campo pode ter `required`, `default`, `help` e (só campo da imagem) `repeat` (a próxima
pendente do pacote vem com o valor da última salva); opções podem ter `code` (forma curta no
nome de arquivo; se uma tem, todas têm). Tipos de objeto: `key`, `label`, `box`, `min`, `max`,
`fields`. O padrão `veiculos` é montado das tabelas do `filename` (mesmos códigos do v2).

## Migrações

Rodam na subida do servidor, são idempotentes e não pedem nada:

| Migração | O que faz |
|---|---|
| `images.MongoRepo.Migrate` | campos soltos + `vehicles` → `fields`/`objects` (update com pipeline, no Mongo) |
| `images.MongoRepo.EnsureIndexes` | índices em `fields.*` (e apaga os antigos) e `{package, ingested_at, _id}` (número da imagem no pacote) |
| `images.Service.MigrateFiles` | verificadas com nome v2 no disco → `<sha1>.ext` |
| `boxes.MongoRepo.EnsureIndexes` | caixas de antes das tarefas → tarefa `padrao` |

Uma migração nova segue o mesmo modelo: idempotente, com teste em `mongo_test.go`, e com
aviso para fazer backup antes da primeira subida.
