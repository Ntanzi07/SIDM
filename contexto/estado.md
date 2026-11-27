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
- **Número da imagem na anotação** ("Imagem 3 / 3.000", `GET /images/{sha1}/position`):
  posição fixa no pacote pela ordem de chegada. Branch `feat/indice-anotacao`.
- **Admin cadastra conta** (`POST /users`, "+ Nova conta" em Configurar → Usuários): nome,
  e-mail, senha e papel; a conta já nasce ativa. Branch `feat/admin-cria-conta`.
- Seta de voltar ao topo em todas as telas (`components/ScrollTop`).
- **Resolução vem da imagem** (`from_image`, ver decisoes.md): `annotation/fromimage.go`,
  `images/size.go`; na tela, só leitura ("Medida do arquivo da imagem"); no editor de perfil,
  a opção "Vem do arquivo da imagem"; no editor de import, sem coluna para ligar. As imagens
  As de antes se corrigem em **Configurar → Armazenamento → Manutenção** (ensaio, depois
  corrigir; `images/fromimage_fix.go`). No banco local (27/11): 1.297 de 3.448 corrigidas, todas
  pendentes sem resolução (nenhuma verificada mudou); backup antes em `backup-resolucao/` (fora
  do git). **Na Jetson:** reconstruir o backend, fazer o backup (convencoes.md) e rodar lá.
- **Dashboard refeito** (`components/Stats`, `SearchPage`): o seletor mostra por perfil as
  verificadas de quantas (com barra), pacotes e datasets (`listPackageSummaries` + datasets,
  dois pedidos); no perfil, 4 destaques (imagens, objetos, caixas, pacotes) que mudam com os
  filtros; **caixas por label** em painéis por tarefa, ordenadas, na cor das caixas da anotação
  (`colorOf`), sem caixas recolhidas; os campos em barras ordenadas (zerados recolhidos,
  "Não informado" apagado, valor único vira uma linha), agrupados em "Na imagem" e por tipo de
  objeto; esqueleto ao contar e barra fina ao filtrar. A rosca (`Donut`) e o CSS dos cartões
  antigos saíram.
- **Esqueleto ao carregar** (`components/Skeleton`): Pacotes, Tarefas e Configurar mostram a
  silhueta da tela (os mesmos cartões na grade escolhida, a barra de busca, o menu e as
  linhas do Configurar) no lugar do anel; aparece depois de 150 ms, brilho parado com
  movimento reduzido. Também o detalhe do pacote (e a lista de imagens dele), a anotação, as
  verificadas, o Dashboard, Exportar, Processar, o detalhe da tarefa, Usuários e Armazenamento,
  com as peças de `components/Skeleton/Shapes.tsx`. O `PageLoader` ficou só em duas esperas
  pequenas (as versões no Dashboard e os pacotes num pop-up das Tarefas).
- **Import com andamento**: com `Accept: application/x-ndjson`, `POST /packages/{name}/import`
  responde uma linha JSON por passo (fases lendo, conferindo, cadastros, gravando) e o
  relatório no fim (`imports/progress.go`); sem o cabeçalho, igual a antes. O diálogo
  "Importar anotações" mostra a etapa, a barra com "N de M" e o tempo (`ImportProgress.tsx`);
  o ensaio pode parar. A criação do pacote mostra o mesmo painel no ensaio e no aplicar do
  CSV. As duas telas usam `runImport` (o `importPackageCsv`/`applyIniProfile` saíram).
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
- **Perfis de importação no mesmo modelo**: lista em cards (`ImportProfileList.tsx`: lê de,
  como acha a imagem, cada ligação "campo ← coluna"); editor com a fonte escolhida em
  "Planilha CSV"/"Dados gravados na imagem", grupos de linhas, arquivo de exemplo e as
  ligações uma por linha com o **valor de exemplo** lido do arquivo (`ImportEditor.css`);
  barra presa embaixo com "N de M campos ligados". A lógica (`mergeRules`/`readRules`) não mudou.
- **Equipamentos e Câmeras** (`RegistrySection.tsx`, saiu do `SettingsPage`): um campo só
  busca e cadastra (sem acento/maiúscula; nome novo vira "Cadastrar", desativado vira
  "Reativar"); renomear na linha. Usuários: grupo vazio (inclusive o de aprovação) não aparece.

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
2. **Configurar**: separar o estado do editor de import do `SettingsPage` num componente,
   como o de anotação (hoje ele pergunta antes de descartar por uma foto do estado, `importSnapshot`).
3. **Dashboard**: contar caixas por valor de atributo (`helmet` com `pessoa = garupa`) e
   filtrar por isso.
4. **Envio do zip**: conferir cada parte (sha256 da parte num header) para pegar dado
   estragado no caminho. O zip de 26 GB que "não abriu" na Jetson estava **bom**: o programa
   que o gerou grava o "total de discos" do localizador zip64 como 0, e o `archive/zip` do Go
   exige 1 (o Python e o 7-Zip abrem). Agora o envio abre por `images/zipopen.go`, que lê esse
   campo como 1 (sem mexer no arquivo); testado com o zip real (68.091 itens). À parte, o
   envio passou a conferir o espaço em disco (`zipspace.go`, 507) e o zip com o fim zerado
   diz que foi o disco. Na Jetson: reconstruir o backend e reenviar (ou "Gravar" de novo o
   envio que falhou, se o zip ainda estiver em `envios/`).
5. Versionamento: o export de uma versão já existe; falta o histórico de atributos das caixas.
6. Processar: testar LPR e eixos com fotos reais (fora do repositório) e ajustar o score.

## Jetson (scs-orin-nano)

- Projeto em `~/workspace/SIDM`; as imagens foram para `/data/sidm/images`
  (`IMAGES_HOST_DIR` no `.env`) porque o `/` (34 GB) encheu.
- Falta mover o Docker inteiro (`/var/lib/docker`, com o volume do Mongo) para
  `/data/docker` (`data-root` no `/etc/docker/daemon.json`, mantendo o runtime da NVIDIA).
- Subir com `docker compose up -d --build` (o `-d` deixa rodar com o terminal fechado).

## Pendências menores

- O `npm run a11y` só confere o pacote, a anotação e as verificadas com `SIDM_A11Y_PACKAGE`
  (ex.: `top`); vale pôr no `a11y.env.local`. (O contraste das etiquetas das caixas, que só aparecia
  assim, foi corrigido.)

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
