package api

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/export"
	"SIDM/internal/filename"
	"SIDM/internal/files"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
	"SIDM/internal/store"
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

	// O lado das imagens e das caixas das operações do catálogo (ver
	// legacy_images.go e store/catalog.go).
	PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error)
	RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error)
	DeletePackageImages(ctx context.Context, name string) (int64, error)
	CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error)
	RenameOnPending(ctx context.Context, reg catalog.Registry, oldName, newName string, now time.Time) (int64, error)
	VerifiedIDs(ctx context.Context, ids []string) (map[string]bool, error)

	InsertImage(ctx context.Context, img *store.Image) error
	DeleteImage(ctx context.Context, sha1 string) error
	DeleteImageHistory(ctx context.Context, sha1 string) error

	// Perfis de export
	CreateExportProfile(ctx context.Context, p *export.Profile) error
	FindExportProfile(ctx context.Context, name string) (*export.Profile, error)
	ReplaceExportProfile(ctx context.Context, p *export.Profile) error
	DeleteExportProfile(ctx context.Context, name string) error
	ListExportProfiles(ctx context.Context) ([]export.Profile, error)
	FindImage(ctx context.Context, sha1 string) (*store.Image, error)
	ReplaceImage(ctx context.Context, img *store.Image, prevUpdatedAt time.Time) error
	ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*store.Image, error)
	ReleaseClaim(ctx context.Context, sha1, user string) error
	ListImages(ctx context.Context, f store.ImageFilter) ([]store.Image, int64, error)
	ExistingImages(ctx context.Context, ids []string) (map[string]store.ExistingImage, error)
	InsertHistory(ctx context.Context, entries []store.HistoryEntry) error

	CreateProfile(ctx context.Context, p *importer.Profile) error
	FindProfile(ctx context.Context, name string) (*importer.Profile, error)
	ReplaceProfile(ctx context.Context, p *importer.Profile) error
	RenameProfile(ctx context.Context, oldName, newName string, now time.Time) error
	DeleteProfile(ctx context.Context, name string) error
	ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error)
	FindImagesByKeys(ctx context.Context, pkg, by string, keys []string) ([]store.Image, error)
}

type Server struct {
	store   Store
	files   *files.Storage
	catalog *catalog.Service
	boxes   *boxes.Service
}

// New monta a API. O catálogo (pacotes, equipamentos, câmeras) e as caixas
// (tarefas, CVAT) já são módulos: as rotas deles entram junto, e os outros
// handlers usam os Services.
func New(s Store, fs *files.Storage, cat *catalog.Service, bx *boxes.Service) *Server {
	return &Server{store: s, files: fs, catalog: cat, boxes: bx}
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
		{"GET", "/codes", s.handleCodes},

		{"POST", "/packages/{name}/next", s.handleNext},
		{"POST", "/packages/{name}/import", s.handleImport},

		{"GET", "/images", s.handleListImages},
		{"POST", "/images", s.handleUpload},
		{"POST", "/images/exists", s.handleExists},
		{"GET", "/images/{sha1}", s.handleGetImage},
		{"PUT", "/images/{sha1}", s.handleAnnotate},
		{"DELETE", "/images/{sha1}", s.handleDeleteImage},
		{"POST", "/images/{sha1}/reopen", s.handleReopen},
		{"GET", "/export-folder-fields", s.handleExportFolderFields},
		{"GET", "/export-profiles", s.handleListExportProfiles},
		{"POST", "/export-profiles", s.handleCreateExportProfile},
		{"GET", "/export-profiles/{name}", s.handleGetExportProfile},
		{"PUT", "/export-profiles/{name}", s.handleReplaceExportProfile},
		{"DELETE", "/export-profiles/{name}", s.handleDeleteExportProfile},
		{"POST", "/exports/preview", s.handleExportPreview},
		{"POST", "/exports/download", s.handleExportDownload},
		{"GET", "/images/{sha1}/file", s.handleImageFile},
		{"POST", "/images/{sha1}/release", s.handleRelease},

		{"POST", "/imports/inspect", s.handleInspect},
		{"GET", "/import-profiles", s.handleListProfiles},
		{"POST", "/import-profiles", s.handleCreateProfile},
		{"GET", "/import-profiles/{name}", s.handleGetProfile},
		{"PUT", "/import-profiles/{name}", s.handleReplaceProfile},
		{"PATCH", "/import-profiles/{name}", s.handleRenameProfile},
		{"DELETE", "/import-profiles/{name}", s.handleDeleteProfile},
	}
	// As rotas dos módulos.
	for _, m := range []platform.Module{s.catalog, s.boxes} {
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

// handleCodes devolve as tabelas fixas do padrão v2 (direção, clima, cor...).
// Equipamentos e câmeras NÃO vêm aqui: podem ser milhares, então têm busca
// própria e paginada (GET /equipments e GET /cameras).
func (s *Server) handleCodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, filename.Codes())
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
