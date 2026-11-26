# Estado atual

Atualizado em 2026-11-26. Branch de trabalho: `feat/perfil_anotacao` (já com a `main` juntada: Processar, LPR e eixos do colega).

## Pronto

### Nesta branch (`feat/perfil_anotacao`, parte ainda sem commit)

- **Herança de perfis de anotação** (`extends`, `annotation/inherit.go`): `moto` estende
  `veiculos`; o banco guarda só o que é do perfil, a API devolve resolvido (`from`,
  `lineage`). Pacotes dos filhos entram nos datasets/pesquisa do pai.
- **Caixa ligada a objeto** (`box_labels` no tipo de lista, `Object.box`, `annotation/links.go`):
  a label (escolhida por tarefa → label no editor) diz campos (`motorcycle` → tipo moto) e
  mapeia atributos do CVAT para campos (`attributes`: `color` → Cor; vale o campo do SIDM).
  O padrão `veiculos` aceita `box_labels` editáveis (guardadas no banco, `annotation/tuning.go`).
- **Ligação automática** no import das caixas e no botão "Ligar caixas aos objetos" do pacote
  (`annotation/autolink.go`, `images/links.go`, `POST /packages/{name}/boxes/link`).
  Na tela, caixas são objetos: o veículo ligado é um card só (borda na cor da caixa), as
  soltas aparecem em "Outros objetos da imagem".
- **Envio de .zip em partes** (`images/zipupload.go`, `hooks/useZipUploads.ts`): partes de
  16 MB, retoma escolhendo o mesmo zip, grava em segundo plano, estado em `envios/<id>.json`.
  Botão único "Adicionar pasta de imagens ou zip" (`FolderOrZip.tsx`).
- **Atalhos de teclado por conta** (`PUT /auth/shortcuts`, `utils/shortcuts.ts`,
  `hooks/useShortcuts.ts`): D/F/Ctrl+Enter/A/P/B/E/Q/Shift+Delete/? na anotação; ajuda com "?".
- **Configurar reorganizado e aberto a todos** (`?aba=` na URL): Conta, Atalhos (todos);
  Perfis, Equipamentos, Câmeras, Importação, Usuários, Armazenamento (admin). Peças
  `pages/Settings/SettingsUI.tsx` (cabeçalho, grupo, linha). Sistema: disco, lixeira
  (`POST /system/trash/purge`), envios parados (`GET /system/storage`).
- Seta de voltar ao topo em todas as telas (`components/ScrollTop`).
- **Acessibilidade (WCAG 2.2 AA) zerada** com `npm run a11y` (axe + puppeteer, usa o Edge
  no Windows; login de admin em `frontend/a11y.env.local`, fora do git). Cores do tema
  ajustadas para ≥ 4,5:1 (`--text-3` nos dois temas; `--accent` e `--success` no claro).
  Atalhos: o nome acessível do botão começa pela tecla visível; "Salvar" nunca fica
  desabilitado (leva à tecla repetida); "Restaurar padrões" só muda o rascunho.
- **Editor de perfil de anotação refeito** (`ProfileEditor.tsx`/`.css`, contas em
  `profileEditorUtils.ts`): lista e editor no estilo de linhas (`SettingsUI`); cada campo é
  uma linha com resumo (tipo, opções, obrigatório) que abre para editar, com **prévia** do
  campo como na anotação (`FieldInput`); opções numa tabela com cabeçalho (nome na tela →
  valor guardado sai dele → código); chave no banco escondida atrás de "Mudar"; `repeat`
  editável; tipo de objeto escolhe "Caixas do CVAT" ou "Lista na anotação"; herdados em
  linhas só de leitura; barra de salvar presa embaixo, pergunta antes de descartar;
  problema do backend abre e leva até o campo. As caixas ligadas (`BoxLabelsEditor`) seguem o
  mesmo padrão: linha com resumo ("motorcycle: Tipo de veículo: Moto"), o que a caixa diz e os
  atributos numa tabela. O CSS `ap-*` antigo saiu.
- **Lista de perfis** (`ProfileList.tsx`/`.css`): um card por perfil com os campos em
  etiquetas (da imagem e de cada tipo de objeto); quem estende outro fica embaixo do pai,
  ligado por um fio, mostrando só o que acrescenta. Ícones no menu do Configurar
  (`SettingsIcons.tsx`, SVG próprio, sem biblioteca). A tela não fala "CVAT": as caixas são
  "caixas marcadas"; o código da opção virou "Sigla" (vai no nome do arquivo exportado e é
  aceito no import; se uma opção tem, todas têm).

### Antes

- Página **Processar** (`/processar`, branch `feat/integracao-ia`, ainda sem commit):
  pré-anotação pelo detector de placas `lpr-class` (módulo `processing`, biblioteca
  `lpr`). Escolhe pacotes e escopo (pendentes/verificadas/todas), grava só caixas na
  tarefa `pre-lpr`, pula o que veio do CVAT ou foi editado; histórico com relatório. Só admin (nem aparece para o usuário normal).
  Testado ponta a ponta com o `lpr-class` de verdade (imagens sintéticas).
  Segundo modelo: o contador de **eixos** (`axlecounter.software`, tarefa `pre-eixos`),
  também testado com o servidor de verdade.
- Tema claro + botão claro/escuro no header (branch `feat/tema-claro`, ainda sem commit).

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
- INI de arquivo com duas imagens (MFR1): lido depois de cada fim de JPEG, então o
  `[Metrologico]` e o `[NMetrologico]` aparecem juntos.
- Data e hora do import sem formato fixo (`importer.ParseDateTime`): data, hora e ms em
  partes (`column`/`time_column`/`ms_column`); o editor de perfil de import grava assim,
  com o formato "Automático" por padrão.
- Perfil de import padrão `splice-ini` (no código, como os de export): já escolhido no
  upload; em Configurar, Ver e Duplicar.
- Equipamento/câmera desativado (apagado em uso) pode ser reativado em Configurar
  (`?inactive=true` na lista); o upload avisa quando o INI cita um desativado.
- `repeat` no campo do perfil de anotação: equipamento, câmera, direção, ângulo e clima
  do `veiculos` repetem a última imagem salva do pacote, sem nada na tela.
- Configurar: perfis de anotação com lista (quantidade no menu) e "Criar novo perfil"
  numa tela própria, como os de import; seletor de chave do INI com busca (`ColumnPicker`).
- Export sem caixas: formato `images` ("Só imagens", perfil padrão `imagens`), sem tarefa
  nem labels; entram as verificadas que passam nos filtros, com o nome de saída (v2 ou
  modelo), por divisão (um grupo só = na raiz) ou por pastas, e um `anotacoes.csv` com os
  dados anotados.

## Próximos passos (em ordem)

1. **Commit da branch**: incluir os arquivos novos (`git status`, os `??`) e rodar
   `go test -p 1 ./...` (sem o `-p 1`, os testes do Mongo estouram o tempo).
2. **Configurar, telas grandes**: levar o editor de perfil de importação e
   Equipamentos/Câmeras para o estilo de linhas (`SettingsUI.tsx`), como já está o de perfil
   de anotação.
3. **Dashboard**: contar caixas por valor de atributo (`helmet` com `pessoa = garupa`) e
   filtrar por isso.
4. **Envio do zip**: conferir cada parte (sha256 da parte num header) para pegar dado
   estragado no caminho; um zip de 26 GB chegou e "não abriu" na Jetson (causa não confirmada:
   testar o zip com 7-Zip e `python3 -c "import zipfile..."` antes de reenviar).
5. Versionamento: o export de uma versão já existe; falta o histórico de atributos das caixas.
6. Processar: testar LPR e eixos com fotos reais (fora do repositório) e ajustar o score.

## Jetson (scs-orin-nano)

- Projeto em `~/workspace/SIDM`; as imagens foram para `/data/sidm/images`
  (`IMAGES_HOST_DIR` no `.env`) porque o `/` (34 GB) encheu.
- Falta mover o Docker inteiro (`/var/lib/docker`, com o volume do Mongo) para
  `/data/docker` (`data-root` no `/etc/docker/daemon.json`, mantendo o runtime da NVIDIA).
- Subir com `docker compose up -d --build` (o `-d` deixa rodar com o terminal fechado).

## Pendências menores

- Recortes das letras da placa (anotar no recorte; decidir se o modelo treina na imagem inteira).
- Mover os testes de cada módulo de `internal/api/` para o próprio módulo.
- Swagger UI embutido (hoje vem de CDN); graceful shutdown.
- Os serviços `backend` e `frontend` do `docker-compose.yml` são imagens: código novo só entra com `docker compose up -d --build <serviço>`.
- Data do import: falta a prévia (endpoint que lê a data em imagens de exemplo, ✔/✘ no
  editor) e um aviso na anotação quando a data não deu para ler.
- Um `splice-ini` salvo no banco antes de virar padrão fica escondido; pode ser apagado.

## Documentos desatualizados (não siga)

- `frontend/STRUCTURE.md`: esboço antigo de pastas (`features/`, `AuthProvider`...) que
  **não existe**. A estrutura real está em `arquitetura.md`.
- `frontend/PLAN.md`: a parte dos rótulos (`data/annotationOptions.ts`, que não existe
  mais) e "equipamento não é cadastrado na anotação" mudaram: a anotação usa os rótulos do perfil e pede confirmação
  para cadastrar um equipamento novo.
- `README.md` → "Decisões de projeto" → "A API depende de uma interface": hoje cada módulo
  tem o seu `Repo` e o `Memory` para os testes.
