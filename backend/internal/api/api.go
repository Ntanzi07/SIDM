package api

// Handlers HTTP.
// Rotas planejadas:
//   GET  /images          busca com filtros (equipment, camera, day_time, weather...)
//   GET  /images/{sha1}   detalhe de uma imagem
//   POST /images          upload de imagens anotadas (valida, renomeia, salva)
//   POST /images/exists   checa hashes antes do upload (evita duplicata)
//   POST /export          gera zip em streaming das imagens selecionadas
//   GET  /codes           tabelas de códigos para os dropdowns do front
