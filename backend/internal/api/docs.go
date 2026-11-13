package api

import (
	_ "embed"
	"net/http"
)

// A especificação vai DENTRO do binário (go:embed): não precisa copiar o
// arquivo junto no deploy, e a doc sempre corresponde à versão do código.
//
//go:embed openapi.yaml
var openAPISpec []byte

// GET /openapi.yaml
func handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Write(openAPISpec)
}

// GET /docs: o Swagger UI.
//
// Os arquivos do Swagger UI vêm de um CDN (unpkg). Se o servidor ficar numa
// rede sem internet, a página não carrega; aí a saída é baixar o
// swagger-ui-dist e embutir também (go:embed), sem mudar mais nada.
func handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(docsHTML))
}

const docsHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <title>SIDM — API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
      persistAuthorization: true
    });
  </script>
</body>
</html>`
