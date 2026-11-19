# Estado atual

Atualizado em 2026-11-19. Branch de trabalho: `feat/filter`.

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
- Atributos nas caixas: a label do CVAT é a chave do tipo de objeto com caixa; o import
  encaixa os atributos no tipo (`ObjectType.FitFields`, relatório com
  `attribute_problems`/`missing_required`/`untyped_labels`/`edited_replaced`); a tela de
  anotação mostra e edita (`PUT /images/{sha1}/boxes/{tarefa}`, com versão).
- Export de várias tarefas: o perfil de export tem `tasks` (o `task` antigo vira `tasks`)
  e `all_tasks` (só as imagens anotadas em todas); as caixas das tarefas são juntadas
  na mesma imagem (`exports.mergeSets`, que também ajusta a escala se o tamanho no CVAT
  diferir).

## Próximos passos (em ordem)

- **Pesquisa/dashboard (Nathan, em andamento):** o backend está pronto
  (`GET /images/stats`, `images/stats.go`, `ImageFilter.Packages`); falta a tela
  `/pesquisa` (escolher o perfil, filtros montados dos campos dele, contagens e barras;
  clicar num número abre a lista em `GET /images` com os mesmos filtros).
- **Export sem caixas (colega da equipe, em andamento):** só os dados anotados de cada
  imagem, sem as caixas do CVAT, no módulo `exports`.
- Depois: levar os atributos das caixas para o export; histórico das edições de atributo.

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
