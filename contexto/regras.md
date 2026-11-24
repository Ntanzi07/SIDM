# Regras que não se quebram

Se um pedido conflitar com uma destas regras, **pare e pergunte** em vez de seguir.

## Segurança e segredos

- **Chaves privadas e senhas nunca vão para o chat, para o código nem para o git.** Não
  leia o conteúdo de chaves privadas nem do `.env`; use as variáveis (`MONGO_ROOT_USER`,
  `MONGO_APP_PASSWORD`...) sem imprimir o valor. O `.env.example` mostra os nomes.
- **PGP não é usado** no projeto. Não sugira nem adicione.
- **Login (módulo `users`):** sessão de 8 horas num cookie HttpOnly; no banco só o hash do
  token e a senha em PBKDF2-SHA256. O `users.Protect` descarta o `X-User` do navegador e
  escreve o da sessão (o resto lê `platform.UserFrom`). **Só admin:** todo `DELETE`, gravar
  perfis de anotação e de import (Configurar) e `/users`. Regra nova de permissão vai em
  `adminOnly` (users/http.go), com teste. Nunca logue senha nem token.
- **Erro interno não vaza:** o cliente recebe "erro interno" (`platform.ServerError`) e o
  detalhe vai para o log.
- **Menor privilégio:** o backend usa o usuário `sidm_app` (só `readWrite` no banco
  `sidm`). O root é só para manutenção e para os testes de integração.

## Dados e arquivos

- **O arquivo original da imagem nunca é alterado.** O INI embutido nos JPEGs dos
  equipamentos é **assinado digitalmente**; mudar um byte invalida a assinatura. O SIDM
  não recomprime, não limpa metadados, não reescreve, não renomeia.
- **No disco, toda imagem é `dataset/<pacote>/<sha1>.ext`**, pendente ou verificada. O nome
  com metadados (v2) fica no banco (`file_name`) e só aparece no export.
- **O campo `Assinatura` do INI não vai para o banco** (só faz sentido junto do arquivo).
- **Fotos reais de placas não entram como fixture de teste** nem no repositório. Testes
  geram imagens sintéticas (`realJPEG(t, ...)` nos testes da API).
- **Imagens e backups não vão para o git:** `/data/` e `backend/backup-antes-perfis/`
  estão no `.gitignore`. Um backup novo dentro do repositório também precisa entrar lá.
- **Banco e disco nunca divergem:** se uma operação mexe nos dois e a segunda parte falha,
  a primeira é desfeita.
- **Antes de uma migração que mexe nos dados de verdade, faça backup** (`mongodump`, ver
  `convencoes.md`) e avise quem vai rodar.

## Processo

- **Um passo de cada vez:** mudanças grandes são quebradas em etapas pequenas, cada uma
  testável e com commit próprio. Não misture refatoração com funcionalidade nova.
- **A API muda junto com o `openapi.yaml`:** o teste `docs_test.go` falha se uma rota não
  estiver documentada.
- **Não faça commit nem push sem a pessoa pedir.** Trabalhe numa branch (`feat/...`,
  `fix/...`, `refactor/...`), nunca direto na `main`.
