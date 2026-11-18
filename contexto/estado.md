# Estado atual

Atualizado em 2026-11-18. Branch de trabalho: `feat/moduloConfigs`.

## Pronto

- Monólito modular completo (`catalog`, `images`, `annotation`, `boxes`, `imports`, `exports`).
- Perfis de anotação: cadastro, validação, editor em **Configurar → Perfis de anotação**,
  perfil por pacote, tela de anotação montada a partir do perfil, migração das imagens.
- Imagens sempre como `<sha1>.ext` no disco; nome v2 só no banco e no export.
- Tarefas e caixas do CVAT; export COCO (RF-DETR), YOLO e Pascal VOC; layout por pastas;
  modelo de nome de saída.

## Próximos passos (em ordem)

1. **Import e export pelos campos do perfil.** Hoje só o `veiculos` funciona de ponta a ponta:
   - os perfis de import (CSV/INI) escrevem nos campos fixos de veículo (`importer.Annotation`);
   - os filtros do export (`export.Filters`) são fixos;
   - as pastas por nível e os campos do modelo de nome no frontend são uma lista fixa
     (`data/annotationOptions.ts`, `FolderLevels.tsx`, `FileNameTemplate.tsx`). O backend
     do export já monta `meta` com os campos de qualquer perfil (`exports.imageMeta`).
2. **Atributos nas caixas:** os tipos de objeto com caixa do perfil ganham campos de
   verdade (importar os atributos do COCO do CVAT, validar pelo perfil, editar na tela).
3. **Export de várias tarefas** (ex.: para-brisa + placa num dataset), com a opção "só
   imagens anotadas em todas as tarefas" para não ensinar que a placa não marcada é fundo.

## Pendências menores

- Recortes das letras da placa (anotar no recorte; decidir se o modelo treina na imagem inteira).
- Mover os testes de cada módulo de `internal/api/` para o próprio módulo.
- Login próprio no lugar do `X-User`; Swagger UI embutido (hoje vem de CDN); graceful shutdown.
- O serviço `backend` do `docker-compose.yml` precisa ser reconstruído para ter o código novo.

## Documentos desatualizados (não siga)

- `frontend/STRUCTURE.md`: esboço antigo de pastas (`features/`, `AuthProvider`...) que
  **não existe**. A estrutura real está em `arquitetura.md`.
- `frontend/PLAN.md`: a parte dos rótulos (`data/annotationOptions.ts`) e "equipamento não
  é cadastrado na anotação" mudaram: a anotação usa os rótulos do perfil e pede confirmação
  para cadastrar um equipamento novo.
- `README.md` → "Decisões de projeto" → "A API depende de uma interface": hoje cada módulo
  tem o seu `Repo` e o `Memory` para os testes.
