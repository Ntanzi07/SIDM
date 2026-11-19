# Decisões

Registro curto do que foi decidido e por quê. Entrada nova no fim. Decisão trocada não é
apagada: marque "**substituída por** <data>".

| Data | Decisão | Motivo |
|---|---|---|
| — | SHA-1 do conteúdo é o `_id` da imagem | duplicata impossível por construção; o hash sai enquanto o arquivo é gravado |
| — | O arquivo original nunca é alterado | o INI dos equipamentos é assinado |
| — | Imagem criptografada é aberta no upload e guardada aberta | hash, duplicata e INI funcionam igual para todas |
| — | A `Assinatura` do INI não vai para o banco | só faz sentido junto do arquivo original |
| — | Sem login: `X-User` | rede interna; login próprio fica para depois |
| — | Perfis de import como dados (JSON no Mongo) | formato novo de CSV é configuração, não código |
| — | Caixas por **tarefa** (projeto do CVAT); import na página da tarefa casando com o banco todo; label desconhecida entra na tarefa | vários projetos (placa, letras, capacete) nas mesmas imagens |
| — | Export escolhe uma tarefa; "negativas" é opção (padrão: não) | o dataset é de um projeto só (**substituída por** 2026-11: várias tarefas) |
| — | Export Pascal VOC e layout por pastas (níveis de metadados com nomes por valor), além do train/valid/test | o dataset antigo da equipe é organizado por tipo/resolução/placa/direção/período |
| — | Modelo de nome de saída configurável no export (v2, padrão antigo, personalizado; precisa ter `{sha1}` ou `{name}`) | cada treino pede um nome |
| 2026-11 | Microsserviços **não**; **monólito modular** | o problema era organização, não escala; um binário é mais simples de manter |
| 2026-11 | Reorganizar módulo por módulo com a API sem mudar (`catalog`, `boxes`, `images`, `imports`, `exports`; o `store` sumiu) | dá para testar e fazer commit a cada passo |
| 2026-11 | Perfil de anotação configurável: campos da imagem + tipos de objeto, com ou sem caixa | projetos diferentes (capacete, eixo, pessoa) com campos diferentes |
| 2026-11 | O perfil é do **pacote** (padrão `veiculos`); troca só sem imagens verificadas | o fluxo é pacote → upload → fila; as verificadas foram conferidas no perfil atual |
| 2026-11 | **Tudo configurável** (os campos de veículo viram o perfil `veiculos`), trocado de uma vez, com migração na subida | um só jeito de anotar; sem código duplicado temporário |
| 2026-11 | Um campo `ref` se chama `equipment` ou `camera` | a cascata de renomear procura o campo pelo nome em qualquer perfil |
| 2026-11 | Valores de fora (CSV, INI, nome v2) passam pelo `Fit`: o que o perfil não tem fica de fora e vira aviso | um valor ruim não derruba o import |
| 2026-11 | Perfil em uso por um pacote não pode ser apagado | a anotação do pacote ficaria sem perfil |
| 2026-11 | **No disco toda imagem é `<sha1>.ext`**; o nome v2 é só um dado no banco; o export escolhe o nome | nenhum perfil precisa saber montar nome de arquivo; o caminho nunca muda |
| 2026-11 | Contexto para IAs em `contexto/`, com ponteiros por ferramenta na raiz | a equipe usa várias IAs e elas precisam seguir as mesmas regras |
| 2026-11 | O perfil de import diz qual perfil de anotação preenche (`annotation_profile`) e é validado contra ele; `vehicles` vira `objects.vehicle` (com `key: plate`) | os destinos de um CSV dependem do que o pacote anota |
| 2026-11 | Perfil de import num pacote de outro perfil de anotação **não é recusado**: o `Transform` usa o perfil do pacote e o resto vai para `outside_profile` | segue a regra do `Fit` (um valor de fora não derruba o import) |
| 2026-11 | O perfil de export tem `annotation_profile` só para os campos de filtros, pastas e nome; **não limita os pacotes** | os padrões (`rf-detr`, `yolo`) servem para qualquer tarefa; imagem de outro perfil só não tem os campos |
| 2026-11 | Label do CVAT = chave do tipo de objeto com caixa; os atributos da caixa são os campos do tipo; label sem tipo fica com os atributos crus | o perfil já dizia isso; sem mapa extra para manter |
| 2026-11 | O reimport do CVAT sobrescreve os atributos editados no SIDM (o ensaio avisa) | o CVAT é a fonte das caixas; casar caixa a caixa seria frágil |
| 2026-11 | Atributo obrigatório de caixa faltando não bloqueia a verificação da imagem | as caixas são por tarefa, com ciclo próprio |
| 2026-11 | Campo de objeto no export/pasta/nome é `<tipo>_<campo>` (do primeiro objeto do tipo); o nome sem o tipo (`plate`) fica como atalho | no `veiculos` os nomes de sempre (`vehicle_type`, `plate_type`) continuam valendo |
| 2026-11 | Export de **várias tarefas** (`tasks`), com as caixas juntas na imagem; `all_tasks` (só as anotadas em todas) é opção, marcada pela tela ao escolher a segunda tarefa; a mesma label em duas tarefas vira uma classe | datasets como para-brisa + placa; sem `all_tasks`, o objeto não marcado viraria fundo |
| 2026-11 | **Pesquisa/dashboard** de imagens fica no módulo `images` (rota de contagem + `memory.go`) e numa tela própria (`/pesquisa`); o export sem labels fica no `exports` e na tela de Exportar. Quem precisar mexer no módulo do outro avisa antes | as duas frentes andam em paralelo sem se chocar nos mesmos arquivos |
| 2026-11 | O `images.ImageFilter` (e o `images.Service.List`) **só cresce**: pode ganhar campos novos (ex.: `Packages []string`), mas nenhum campo existente muda de nome ou de sentido | o export e a pesquisa dependem do mesmo filtro |
| 2026-11 | Um formato de filtro só: `fields.<campo>` e `objects.<campo>`, com os valores no tipo do perfil; nada de formato novo na pesquisa nem no export | "exportar o que eu pesquisei" vira passar os mesmos filtros |
| 2026-11 | Imagens de um perfil de anotação = imagens dos pacotes que usam o perfil (`catalog.PackagesWithProfile`); a pesquisa é sempre de **um perfil** por vez | o perfil é do pacote, não da imagem; misturar perfis (rosto e carro) não faz sentido |
| 2026-11 | A pesquisa conta só as **verificadas** por padrão; pendentes só como opção explícita (igual ao export) | rascunho pode estar incompleto |
| 2026-11 | Arquivos compartilhados (`services/api.ts`, `router.tsx`/menu, `openapi.yaml`, `README.md`, `contexto/`): cada um edita a seção do seu módulo e **adiciona** linhas, sem reescrever as do outro | é onde o merge dá conflito |
| 2026-11 | Uma branch por frente, saindo da `main` atualizada (a `feat/filter` entra antes); quem termina primeiro faz o merge, o outro faz rebase | evita juntar duas frentes grandes de uma vez |
