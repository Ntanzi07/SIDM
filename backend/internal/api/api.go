// Package api junta os módulos num servidor HTTP só: registra as rotas de
// cada um, o /health e a documentação (/docs e /openapi.yaml). Não tem regra
// de negócio: cada assunto está no módulo dele (catalog, images, boxes,
// imports, exports).
package api

import (
	"context"
	"net/http"

	"SIDM/internal/platform"
)

// A lista de rotas é a tabela de routes() (o /health e as dos módulos), e
// cada uma está documentada no openapi.yaml (o teste em docs_test.go
// confere). Toda listagem responde no mesmo formato: {"total": N,
// "items": [...]}, com ?limit= e ?skip= para paginar.

type Server struct {
	ping    func(ctx context.Context) error
	modules []platform.Module
}

// New monta a API. ping é o teste do banco do /health.
func New(ping func(ctx context.Context) error, modules ...platform.Module) *Server {
	return &Server{ping: ping, modules: modules}
}

// route é uma linha da tabela de rotas.
type route struct {
	method, path string
	handler      http.HandlerFunc
}

// routes é a tabela de TODAS as rotas da API. Fica separada do Routes()
// para o teste conseguir percorrer a lista e conferir que cada rota está
// documentada no openapi.yaml (ver docs_test.go).
func (s *Server) routes() []route {
	rts := []route{{"GET", "/health", s.handleHealth}}
	for _, m := range s.modules {
		for _, rt := range m.Routes() {
			rts = append(rts, route{rt.Method, rt.Path, rt.Handler})
		}
	}
	return rts
}

// Routes registra todas as rotas, mais a documentação, e devolve o handler
// pronto para o http.ListenAndServe.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.method+" "+rt.path, rt.handler)
	}
	mux.HandleFunc("GET /docs", handleDocs)
	mux.HandleFunc("GET /openapi.yaml", handleOpenAPI)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// r.Context() é cancelado se o cliente desistir da requisição.
	if err := s.ping(r.Context()); err != nil {
		http.Error(w, "mongo indisponível", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}
