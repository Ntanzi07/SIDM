# Contexto do SIDM para IAs (e para gente)

Esta pasta é a **fonte única** do que uma IA precisa saber para trabalhar no SIDM sem
contradizer o que já foi decidido. Cada ferramenta tem um arquivo curto na raiz, no nome
que ela procura, que só aponta para cá:

| Ferramenta | Arquivo na raiz |
|---|---|
| Codex, Cursor, Copilot (agente), Aider e as que seguem o padrão `AGENTS.md` | `AGENTS.md` |
| Claude Code | `CLAUDE.md` (importa o `AGENTS.md`) |
| Gemini CLI | `GEMINI.md` |
| GitHub Copilot (chat e revisão) | `.github/copilot-instructions.md` |
| Cursor (regras do projeto) | `.cursor/rules/sidm.mdc` |

Não escreva regras nesses arquivos da raiz: escreva **aqui** e deixe eles só apontando.
Duas cópias da mesma regra sempre acabam divergindo.

## Ordem de leitura

1. [regras.md](regras.md): o que **nunca** pode acontecer (ler sempre, antes de tudo).
2. [projeto.md](projeto.md): o que é o SIDM, quem usa, o fluxo.
3. [arquitetura.md](arquitetura.md): os módulos e como eles conversam.
4. [dados.md](dados.md): coleções do Mongo, o documento da imagem, o disco.
5. [convencoes.md](convencoes.md): como escrever código, testes e commits aqui.
6. [glossario.md](glossario.md): os termos (há três tipos de "perfil"!).
7. [decisoes.md](decisoes.md): por que as coisas são como são.
8. [estado.md](estado.md): o que está pronto, o que falta, o que está desatualizado.

O `README.md` da raiz é o manual completo (API, formatos, exemplos). Esta pasta é o
resumo para orientar o trabalho; quando os dois discordarem, o código e os testes
decidem, e o que estiver errado é corrigido.

## Como manter (vale para pessoas e IAs)

- **Decidiu algo que muda o desenho?** Acrescente uma entrada em `decisoes.md` (data,
  decisão, motivo). Não apague as antigas: marque como "substituída por".
- **Terminou ou começou uma etapa?** Atualize `estado.md`.
- **Mudou um módulo, coleção ou regra?** Atualize o arquivo do assunto no mesmo commit
  da mudança.
- Escreva em português, curto e no presente ("o export usa", não "o export vai usar").
