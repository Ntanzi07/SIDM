# Estado atual

Atualizado em 2026-11-18. Branch de trabalho: `feat/moduloConfigs`.

## Pronto

- Monólito modular completo (`catalog`, `images`, `annotation`, `boxes`, `imports`, `exports`).
- Perfis de anotação: cadastro, validação, editor em **Configurar → Perfis de anotação**,
  perfil por pacote, tela de anotação montada a partir do perfil, migração das imagens.
- Imagens sempre como `<sha1>.ext` no disco; nome v2 só no banco e no export.
- Tarefas e caixas do CVAT; export COCO (RF-DETR), YOLO e Pascal VOC; layout por pastas;
  modelo de nome de saída.
- Import e export pelos campos do perfil de anotação: o perfil de import tem
  `annotation_profile` e preenche `fields`/`objects.<tipo>` dele (o formato antigo
  `vehicles` ainda é aceito); o perfil de export tem `annotation_profile`, filtros
  genéricos (`filters.fields`/`filters.objects`) e pastas/nome calculados do perfil
  (`export.MetaFields`, `GET /export-folder-fields?annotation_profile=`). As telas de
  Configurar e Exportar montam os campos a partir do perfil.

## Próximos passos (em ordem)

1. **Atributos nas caixas:** os tipos de objeto com caixa do perfil ganham campos de
   verdade (importar os atributos do COCO do CVAT, validar pelo perfil, editar na tela).
2. **Export de várias tarefas** (ex.: para-brisa + placa num dataset), com a opção "só
   imagens anotadas em todas as tarefas" para não ensinar que a placa não marcada é fundo.

## Pendências menores

- Recortes das letras da placa (anotar no recorte; decidir se o modelo treina na imagem inteira).
- Mover os testes de cada módulo de `internal/api/` para o próprio módulo.
- Login próprio no lugar do `X-User`; Swagger UI embutido (hoje vem de CDN); graceful shutdown.
- O serviço `backend` do `docker-compose.yml` precisa ser reconstruído para ter o código novo.

## Documentos desatualizados (não siga)

- `frontend/STRUCTURE.md`: esboço antigo de pastas (`features/`, `AuthProvider`...) que
  **não existe**. A estrutura real está em `arquitetura.md`.
- `frontend/PLAN.md`: a parte dos rótulos (`data/annotationOptions.ts`, que não existe
  mais) e "equipamento não é cadastrado na anotação" mudaram: a anotação usa os rótulos do perfil e pede confirmação
  para cadastrar um equipamento novo.
- `README.md` → "Decisões de projeto" → "A API depende de uma interface": hoje cada módulo
  tem o seu `Repo` e o `Memory` para os testes.
