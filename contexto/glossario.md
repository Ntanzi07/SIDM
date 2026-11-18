# Glossário

| Termo | Significado |
|---|---|
| **pacote** | lote de imagens (`packages`); vira a pasta `dataset/<pacote>/`; tem um perfil de anotação |
| **imagem** | um arquivo, identificado pelo SHA-1 do conteúdo |
| **pendente / rascunho** | `status: pending`: ainda não conferida; os campos são rascunho |
| **verificada** | `status: verified`: alguém conferiu tudo; só ela entra no export |
| **fila** | a ordem de anotação; cada imagem fica **reservada** 15 min para quem pegou |
| **reabrir** | uma verificada volta a pendente para ser editada |
| **perfil de anotação** | o que se anota nas imagens de um pacote: campos e tipos de objeto (`annotation`) |
| **perfil de import** | como ler um CSV ou o INI e traduzir para os campos (`imports`/`importer`) |
| **perfil de export** | formato, divisão, filtros, pastas e nomes do dataset (`exports`/`export`) |
| **campo** | um dado da imagem (`fields.day_time`) ou de um objeto (`objects[0].plate`) |
| **objeto sem caixa** | uma lista na imagem, preenchida na anotação (ex.: veículos com placa) |
| **objeto com caixa** | uma label do CVAT; cada caixa é um objeto (ex.: capacete) |
| **tarefa** | um projeto de caixas do CVAT (`annotation_tasks`); a mesma imagem pode estar em várias |
| **negativa** | imagem anotada numa tarefa **sem** nenhuma caixa (entra no export como exemplo vazio, se o perfil pedir) |
| **nome v2** | `<sha1>_v2_<equip>_<câmera>_<códigos>_<resolução>_<data>_<veículos>.ext`; calculado no perfil `veiculos`, guardado em `file_name`, usado no export |
| **padrão antigo** | o modelo de nome de antes do v2 (`{sha1}{direction}...{plate}`), oferecido no export |
| **código** | a forma curta de um valor no nome do arquivo (`night` → `N`); a API sempre usa o valor |
| **INI embutido** | os metadados que os equipamentos gravam no fim do JPEG (assinados) |
| **raio-x** | `POST /imports/inspect`: colunas e valores de um arquivo, para escrever um perfil |
| **ensaio** | `dry_run=true` (padrão do import): mostra o relatório sem gravar |
