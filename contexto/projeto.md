# O projeto

**SIDM** controla e anota imagens para montar datasets de treino de modelos de visão
computacional (detecção de placas, de veículos, de capacete, de eixos...). Uso interno da
Splice, em rede interna.

## O fluxo

```
criar pacote ─▶ upload ─▶ (import de CSV/INI) ─▶ fila de anotação ─▶ verificar ─▶ export
(com perfil     (sha1 =    (pré-preenche o      (cada pessoa       (confere      (zip RF-DETR,
 de anotação)    _id)       rascunho)            pega uma)          tudo)         YOLO, VOC)
                                     ▲
              caixas do CVAT (por tarefa) ── import do COCO
```

- **Pacote:** um lote de imagens (uma coleta, um equipamento, um mês...). Cada pacote tem
  um **perfil de anotação**, que diz o que se anota nas imagens dele.
- **Imagem:** identificada pelo **SHA-1 do conteúdo** (duplicata é impossível). Começa
  `pending` (rascunho) e vira `verified` quando alguém confere tudo.
- **Tarefa de anotação:** um projeto de caixas (bounding boxes) feito no CVAT (ex.:
  "placa", "capacete"). A mesma imagem pode ter caixas em várias tarefas. O COCO do CVAT
  é importado na página da tarefa e casa com as imagens do banco todo.
- **Export:** escolhe uma tarefa e um perfil de export e gera o zip do dataset, só com as
  imagens verificadas e anotadas naquela tarefa. O formato **só imagens** (`images`) não
  usa tarefa: leva as verificadas renomeadas e um `anotacoes.csv` com os dados anotados.

## Stack

| Parte | Tecnologia |
|---|---|
| Backend | Go (ver `backend/go.mod`), `net/http` puro, driver oficial do Mongo v2 |
| Banco | MongoDB 8 (sem replica set: **não há transações**) |
| Frontend | React 19 + TypeScript + Vite, `react-router-dom`, CSS por página (sem framework de UI) |
| Infra | `docker-compose.yml` (Mongo + backend); o frontend roda com `npm run dev` |
| Docs da API | `backend/internal/api/openapi.yaml`, servido em `/docs` |

## Pessoas e uso

- Quem anota: a equipe de engenharia, no navegador, na rede interna.
- O nome de quem está usando vai no `X-User` (histórico, reserva na fila).
- O idioma de tudo (código comentado, mensagens de erro, telas, commits) é **português**.
