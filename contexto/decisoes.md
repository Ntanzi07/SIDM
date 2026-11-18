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
| — | Export escolhe uma tarefa; "negativas" é opção (padrão: não) | o dataset é de um projeto só |
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
