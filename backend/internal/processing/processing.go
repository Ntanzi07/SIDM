// Package processing é o módulo da pré-anotação por modelos de IA: um
// modelo (o detector de placas LPR, o contador de eixos) passa pelas imagens dos pacotes
// escolhidos e grava as caixas que achou numa tarefa, para uma pessoa só
// corrigir (no CVAT ou na tela).
//
// Cada modelo é um adaptador (Model): lpr.go e axle.go. Um modelo novo é
// mais um adaptador, ligado no main.go.
//
// O módulo é dono da coleção processing_runs (os processamentos, com o
// progresso e o relatório). As imagens vêm do images (Images), o arquivo do
// disco (Files) e as caixas são gravadas pelo boxes (Boxes), que nunca
// passa por cima do que veio do CVAT ou foi editado.
package processing

import (
	"context"
	"time"

	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/images"
)

// ModelInfo é o que a tela mostra de um modelo.
type ModelInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// DefaultTask: a tarefa onde as caixas vão, se o pedido não disser outra.
	DefaultTask string `json:"default_task"`
	// Labels: as labels que o modelo pode gerar (entram na tarefa).
	Labels []string `json:"labels"`
	// DefaultMinScore: o score mínimo se o pedido não disser outro.
	DefaultMinScore float64 `json:"default_min_score"`
	// ScoreHelp: a que o score mínimo se aplica (o texto embaixo do campo).
	ScoreHelp string `json:"score_help"`
}

// Options são os ajustes de um processamento que o modelo usa.
type Options struct {
	// MinScore: abaixo dele, o que o modelo achou fica de fora (cada modelo
	// diz a quê se aplica: ModelInfo.ScoreHelp).
	MinScore float64
}

// Model é um modelo de IA que acha caixas numa imagem.
type Model interface {
	Info() ModelInfo
	// Ping: o modelo está de pé (rápido: a tela pergunta a cada visita).
	Ping(ctx context.Context) error
	// Detect recebe os bytes do arquivo (que nunca muda) e devolve as caixas
	// em pixels da imagem.
	Detect(ctx context.Context, img []byte, opt Options) ([]boxes.Box, error)
}

// Os escopos: quais imagens dos pacotes entram.
const (
	ScopePending  = "pending"
	ScopeVerified = "verified"
	ScopeAll      = "all"
)

// Os status de um processamento.
const (
	StatusRunning     = "running"
	StatusDone        = "done"
	StatusFailed      = "failed"      // o modelo caiu e não voltou
	StatusCanceled    = "canceled"    // alguém cancelou
	StatusInterrupted = "interrupted" // o servidor do SIDM reiniciou no meio
)

// Run é um processamento: o pedido, o progresso e o relatório.
type Run struct {
	ID       string   `bson:"_id" json:"id"`
	Model    string   `bson:"model" json:"model"`
	Packages []string `bson:"packages" json:"packages"`
	Scope    string   `bson:"scope" json:"scope"`
	Task     string   `bson:"task" json:"task"`
	MinScore float64  `bson:"min_score" json:"min_score"`

	Status string `bson:"status" json:"status"`
	// Message: o motivo de um failed (o erro do modelo).
	Message string `bson:"message,omitempty" json:"message,omitempty"`
	Total   int    `bson:"total" json:"total"`     // imagens que entraram
	Done    int    `bson:"done" json:"done"`       // já passaram pelo modelo (com ou sem erro)
	Written int    `bson:"written" json:"written"` // imagens com caixas gravadas
	Boxes   int    `bson:"boxes" json:"boxes"`     // caixas gravadas
	// Skipped: imagens puladas porque as caixas da tarefa vieram do CVAT ou
	// foram editadas (SkippedIDs guarda as primeiras).
	Skipped    int        `bson:"skipped" json:"skipped"`
	SkippedIDs []string   `bson:"skipped_ids,omitempty" json:"skipped_ids,omitempty"`
	ErrorCount int        `bson:"error_count" json:"error_count"`
	Errors     []RunError `bson:"errors,omitempty" json:"errors,omitempty"` // as primeiras

	StartedBy  string     `bson:"started_by" json:"started_by"`
	StartedAt  time.Time  `bson:"started_at" json:"started_at"`
	FinishedAt *time.Time `bson:"finished_at,omitempty" json:"finished_at,omitempty"`
}

// RunError: uma imagem que deu erro.
type RunError struct {
	SHA1 string `bson:"sha1" json:"sha1"`
	Msg  string `bson:"msg" json:"msg"`
}

// RunInput é o pedido de um processamento.
type RunInput struct {
	Model    string   `json:"model"`
	Packages []string `json:"packages"`
	Scope    string   `json:"scope"`
	Task     string   `json:"task"`
	MinScore *float64 `json:"min_score"`
}

// Repo é a coleção processing_runs.
type Repo interface {
	CreateRun(ctx context.Context, r *Run) error
	SaveRun(ctx context.Context, r *Run) error
	FindRun(ctx context.Context, id string) (*Run, error)
	// ListRuns: os mais novos primeiro.
	ListRuns(ctx context.Context, limit int64) ([]Run, error)
	// InterruptRunning: os que estavam rodando viram interrupted.
	InterruptRunning(ctx context.Context, now time.Time) (int64, error)
}

// Images é o que o módulo usa das imagens.
type Images interface {
	List(ctx context.Context, f images.ImageFilter) ([]images.Image, int64, error)
}

// Packages: os pacotes do pedido têm de existir.
type Packages interface {
	FindPackage(ctx context.Context, name string) (*catalog.Package, error)
}

// Files: onde está o arquivo de uma imagem (o *files.Storage).
type Files interface {
	Abs(path string) string
}

// Boxes é o que o módulo usa das caixas.
type Boxes interface {
	EnsureTask(ctx context.Context, in boxes.TaskInput, user string) error
	PutModelSets(ctx context.Context, sets []boxes.Set, tool, user string) (int, []string, error)
}
