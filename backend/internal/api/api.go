package api

import (
	"context"
	"net/http"
	"net/url"

	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/export"
	"SIDM/internal/files"
	"SIDM/internal/images"
	"SIDM/internal/imports"
	"SIDM/internal/platform"
)

// A lista de rotas é a tabela de routes() (as daqui e as dos módulos), e
// cada uma está documentada no openapi.yaml (o teste em docs_test.go
// confere). Toda listagem responde no mesmo formato: {"total": N,
// "items": [...]}, com ?limit= e ?skip= para paginar.

// Store é tudo que a API precisa do banco.
// A API depende desta interface, e não do *store.Store concreto: assim os
// testes usam um banco em memória (ver api_test.go) e não precisam de Mongo.
// O *store.Store satisfaz a interface sem declarar nada (interfaces em Go
// são implícitas): basta ter os métodos.
type Store interface {
	Ping(ctx context.Context) error

	// Perfis de export
	CreateExportProfile(ctx context.Context, p *export.Profile) error
	FindExportProfile(ctx context.Context, name string) (*export.Profile, error)
	ReplaceExportProfile(ctx context.Context, p *export.Profile) error
	DeleteExportProfile(ctx context.Context, name string) error
	ListExportProfiles(ctx context.Context) ([]export.Profile, error)
}

type Server struct {
	store   Store
	files   *files.Storage
	catalog *catalog.Service
	boxes   *boxes.Service
	images  *images.Service
	imports *imports.Service
}

// New monta a API. Catálogo, caixas, imagens e import já são módulos: as
// rotas deles entram junto, e os handlers daqui (o export) usam os Services.
func New(s Store, fs *files.Storage, cat *catalog.Service, bx *boxes.Service, img *images.Service, imp *imports.Service) *Server {
	return &Server{store: s, files: fs, catalog: cat, boxes: bx, images: img, imports: imp}
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
	rts := []route{
		{"GET", "/health", s.handleHealth},

		{"GET", "/export-folder-fields", s.handleExportFolderFields},
		{"GET", "/export-profiles", s.handleListExportProfiles},
		{"POST", "/export-profiles", s.handleCreateExportProfile},
		{"GET", "/export-profiles/{name}", s.handleGetExportProfile},
		{"PUT", "/export-profiles/{name}", s.handleReplaceExportProfile},
		{"DELETE", "/export-profiles/{name}", s.handleDeleteExportProfile},
		{"POST", "/exports/preview", s.handleExportPreview},
		{"POST", "/exports/download", s.handleExportDownload},
	}
	// As rotas dos módulos.
	for _, m := range []platform.Module{s.catalog, s.boxes, s.images, s.imports} {
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
	if err := s.store.Ping(r.Context()); err != nil {
		http.Error(w, "mongo indisponível", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// page é o formato padrão de toda listagem.
type page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
}

// Os ajudantes de resposta ficam no platform; estes atalhos ficam até os
// handlers daqui irem para os módulos.
func (s *Server) serverError(w http.ResponseWriter, err error) { platform.ServerError(w, err) }

func (s *Server) logError(err error) { platform.LogError(err) }

func userFrom(r *http.Request) string { return platform.UserFrom(r) }

func writeJSON(w http.ResponseWriter, status int, v any) { platform.WriteJSON(w, status, v) }

func writeError(w http.ResponseWriter, status int, msg string) { platform.WriteError(w, status, msg) }

func intParam(q url.Values, key string, def, minV, maxV int64) int64 {
	return platform.IntParam(q, key, def, minV, maxV)
}

// packageByName: ver catalog.Service.PackageByName.
func (s *Server) packageByName(w http.ResponseWriter, r *http.Request, name string) (*catalog.Package, bool) {
	return s.catalog.PackageByName(w, r, name)
}
