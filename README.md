# SIDM

Sistema de controle e anotação de imagens de veículos.

## Estrutura
```
backend/    API em Go (parser do nome, Mongo, HTTP)
frontend/   React (abas Busca e Anotação)
mongo/init/ script de criação das collections e índices
```

## Rodando
```bash
cp .env.example .env
docker compose up --build
```
Backend em http://localhost:8080/health
