package processing

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // DecodeConfig: o tamanho de JPEG e PNG
	_ "image/png"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"SIDM/internal/boxes"
	"SIDM/internal/images"
	"SIDM/internal/platform"
)

const (
	batchSize   = 20  // imagens por gravação (e por atualização do progresso)
	maxListed   = 50  // erros e puladas guardados no relatório
	maxFailures = 5   // erros seguidos antes de conferir se o modelo caiu
	maxPackages = 200 // pacotes num pedido
	listPage    = 500
)

// InvalidError: o pedido não passa na validação (400).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

// ConflictError: já há um processamento rodando no modelo, ou o
// processamento já terminou (409).
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// UnavailableError: o modelo não responde (503).
type UnavailableError struct{ Msg string }

func (e *UnavailableError) Error() string { return e.Msg }

// Service são as regras do módulo. Os processamentos rodam em segundo
// plano, um por modelo (o servidor do LPR atende uma imagem por vez).
type Service struct {
	repo     Repo
	images   Images
	packages Packages
	files    Files
	boxes    Boxes
	models   []Model

	mu     sync.Mutex
	active map[string]*job // por modelo
	wg     sync.WaitGroup
	now    func() time.Time
}

// job é um processamento rodando: o Run (lido pela tela enquanto anda,
// sempre com s.mu) e como cancelar.
type job struct {
	run    Run
	cancel context.CancelFunc
}

func New(repo Repo, imgs Images, packages Packages, files Files, bx Boxes, models ...Model) *Service {
	return &Service{repo: repo, images: imgs, packages: packages, files: files, boxes: bx, models: models,
		active: map[string]*job{},
		now:    func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }}
}

// ModelStatus é um modelo com o estado dele agora.
type ModelStatus struct {
	ModelInfo
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
	Busy      bool   `json:"busy"` // há um processamento rodando nele
}

// Models: os modelos ligados, cada um com o Ping (prazo de 1 s, todos ao
// mesmo tempo). O detalhe de um modelo fora do ar vai para o log.
func (s *Service) Models(ctx context.Context) []ModelStatus {
	out := make([]ModelStatus, len(s.models))
	var wg sync.WaitGroup
	for i, m := range s.models {
		out[i] = ModelStatus{ModelInfo: m.Info(), Available: true}
		wg.Add(1)
		go func() {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			if err := m.Ping(pctx); err != nil {
				log.Printf("modelo %s indisponível: %v", out[i].Key, err)
				out[i].Available, out[i].Error = false, "o modelo não está respondendo"
			}
		}()
	}
	wg.Wait()
	s.mu.Lock()
	for i := range out {
		out[i].Busy = s.active[out[i].Key] != nil
	}
	s.mu.Unlock()
	return out
}

func (s *Service) model(key string) Model {
	for _, m := range s.models {
		if m.Info().Key == key {
			return m
		}
	}
	return nil
}

// normalize valida o pedido e completa o que veio vazio.
func (s *Service) normalize(ctx context.Context, in *RunInput) (Model, error) {
	in.Model = strings.TrimSpace(in.Model)
	m := s.model(in.Model)
	if m == nil {
		return nil, &InvalidError{fmt.Sprintf("modelo %q não existe ou não está ligado", in.Model)}
	}
	seen := map[string]bool{}
	pkgs := make([]string, 0, len(in.Packages))
	for _, p := range in.Packages {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		return nil, &InvalidError{"escolha pelo menos um pacote"}
	}
	if len(pkgs) > maxPackages {
		return nil, &InvalidError{fmt.Sprintf("no máximo %d pacotes por processamento", maxPackages)}
	}
	for _, p := range pkgs {
		if _, err := s.packages.FindPackage(ctx, p); errors.Is(err, platform.ErrNotFound) {
			return nil, &InvalidError{fmt.Sprintf("o pacote %q não existe", p)}
		} else if err != nil {
			return nil, err
		}
	}
	in.Packages = pkgs
	switch in.Scope {
	case "":
		in.Scope = ScopePending
	case ScopePending, ScopeVerified, ScopeAll:
	default:
		return nil, &InvalidError{"imagens: use pending, verified ou all"}
	}
	in.Task = strings.TrimSpace(in.Task)
	if in.Task == "" {
		in.Task = m.Info().DefaultTask
	}
	if in.MinScore == nil {
		v := m.Info().DefaultMinScore
		in.MinScore = &v
	}
	if *in.MinScore < 0 || *in.MinScore > 1 {
		return nil, &InvalidError{"o score mínimo vai de 0 a 1"}
	}
	return m, nil
}

// Start valida o pedido, prepara a tarefa e solta o processamento em
// segundo plano. Devolve o Run recém-criado (status running).
func (s *Service) Start(ctx context.Context, in RunInput, user string) (*Run, error) {
	m, err := s.normalize(ctx, &in)
	if err != nil {
		return nil, err
	}
	info := m.Info()
	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err = m.Ping(pctx)
	cancel()
	if err != nil {
		log.Printf("modelo %s indisponível: %v", info.Key, err)
		return nil, &UnavailableError{fmt.Sprintf("o modelo %s não está respondendo", info.Label)}
	}
	if err := s.boxes.EnsureTask(ctx, boxes.TaskInput{
		Name:        in.Task,
		Description: "Pré-anotação do modelo " + info.Label,
		Labels:      info.Labels,
	}, user); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.active[info.Key] != nil {
		s.mu.Unlock()
		return nil, &ConflictError{fmt.Sprintf("já há um processamento rodando no modelo %s: espere terminar ou cancele", info.Label)}
	}
	run := Run{
		ID: newID(s.now()), Model: info.Key, Packages: in.Packages, Scope: in.Scope, Task: in.Task, MinScore: *in.MinScore,
		Status: StatusRunning, StartedBy: user, StartedAt: s.now(),
	}
	jctx, jcancel := context.WithCancel(context.Background())
	j := &job{run: run, cancel: jcancel}
	s.active[info.Key] = j
	s.mu.Unlock()

	if err := s.repo.CreateRun(ctx, &run); err != nil {
		s.mu.Lock()
		delete(s.active, info.Key)
		s.mu.Unlock()
		jcancel()
		return nil, err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.work(jctx, m, j, user)
	}()
	return &run, nil
}

// Wait espera os processamentos rodando terminarem (testes).
func (s *Service) Wait() { s.wg.Wait() }

// Cancel pede para parar um processamento rodando. Ele para depois da
// imagem que está no modelo; o que já foi gravado fica.
func (s *Service) Cancel(ctx context.Context, id string) (*Run, error) {
	s.mu.Lock()
	for _, j := range s.active {
		if j.run.ID == id {
			j.cancel()
			run := j.snapshot()
			s.mu.Unlock()
			return &run, nil
		}
	}
	s.mu.Unlock()
	run, err := s.repo.FindRun(ctx, id)
	if err != nil {
		return nil, err
	}
	return nil, &ConflictError{fmt.Sprintf("o processamento já terminou (%s)", run.Status)}
}

// FindRun: o rodando vem da memória (o progresso mais novo).
func (s *Service) FindRun(ctx context.Context, id string) (*Run, error) {
	s.mu.Lock()
	for _, j := range s.active {
		if j.run.ID == id {
			run := j.snapshot()
			s.mu.Unlock()
			return &run, nil
		}
	}
	s.mu.Unlock()
	return s.repo.FindRun(ctx, id)
}

// ListRuns: os últimos, mais novos primeiro (os rodando com o progresso da
// memória).
func (s *Service) ListRuns(ctx context.Context, limit int64) ([]Run, error) {
	runs, err := s.repo.ListRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range runs {
		for _, j := range s.active {
			if j.run.ID == runs[i].ID {
				runs[i] = j.snapshot()
			}
		}
	}
	return runs, nil
}

// MarkInterrupted roda na subida: o que estava rodando quando o servidor
// caiu vira interrupted (rodar de novo é seguro: só o que é do próprio
// modelo é regravado).
func (s *Service) MarkInterrupted(ctx context.Context) (int64, error) {
	return s.repo.InterruptRunning(ctx, s.now())
}

func (j *job) snapshot() Run {
	run := j.run
	run.Packages = append([]string(nil), j.run.Packages...)
	run.Errors = append([]RunError(nil), j.run.Errors...)
	run.SkippedIDs = append([]string(nil), j.run.SkippedIDs...)
	return run
}

// work é o processamento em si: lista as imagens, passa cada uma pelo
// modelo e grava as caixas em lotes.
func (s *Service) work(ctx context.Context, m Model, j *job, user string) {
	key := m.Info().Key
	defer func() {
		s.mu.Lock()
		delete(s.active, key)
		s.mu.Unlock()
		j.cancel()
	}()
	// as gravações do Run não usam o ctx do job: um cancelamento ainda
	// precisa ser gravado
	save := func() {
		s.mu.Lock()
		run := j.snapshot()
		s.mu.Unlock()
		if err := s.repo.SaveRun(context.Background(), &run); err != nil {
			log.Printf("processamento %s: falha ao gravar o progresso: %v", run.ID, err)
		}
	}
	update := func(f func(r *Run)) {
		s.mu.Lock()
		f(&j.run)
		s.mu.Unlock()
	}
	finish := func(status, msg string) {
		now := s.now()
		update(func(r *Run) { r.Status, r.Message, r.FinishedAt = status, msg, &now })
		save()
	}

	s.mu.Lock()
	in := j.snapshot()
	s.mu.Unlock()

	list, err := s.list(ctx, in)
	if err != nil {
		if ctx.Err() != nil {
			finish(StatusCanceled, "")
			return
		}
		log.Printf("processamento %s: falha ao listar as imagens: %v", in.ID, err)
		finish(StatusFailed, "erro ao listar as imagens")
		return
	}
	update(func(r *Run) { r.Total = len(list) })
	save()

	opt := Options{MinScore: in.MinScore}
	source := boxes.Source{File: "run:" + in.ID}
	var pending []boxes.Set
	sinceSave, failures := 0, 0

	flush := func() error {
		if len(pending) > 0 {
			written, skipped, err := s.boxes.PutModelSets(context.Background(), pending, key, user)
			if err != nil {
				return err
			}
			skip := map[string]bool{}
			for _, id := range skipped {
				skip[id] = true
			}
			n := 0
			for _, set := range pending {
				if !skip[set.ImageID] {
					n += len(set.Boxes)
				}
			}
			update(func(r *Run) {
				r.Written += written
				r.Boxes += n
				r.Skipped += len(skipped)
				for _, id := range skipped {
					if len(r.SkippedIDs) < maxListed {
						r.SkippedIDs = append(r.SkippedIDs, id)
					}
				}
			})
			pending = pending[:0]
		}
		sinceSave = 0
		save()
		return nil
	}
	fail := func(sha1, msg string) {
		update(func(r *Run) {
			r.ErrorCount++
			if len(r.Errors) < maxListed {
				r.Errors = append(r.Errors, RunError{SHA1: sha1, Msg: msg})
			}
		})
	}

	for _, img := range list {
		if ctx.Err() != nil {
			break
		}
		set, err := s.detect(ctx, m, img, opt)
		switch {
		case ctx.Err() != nil:
			// cancelado no meio: a imagem não conta
		case err != nil:
			fail(img.SHA1, err.Error())
			failures++
			if failures >= maxFailures {
				pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				perr := m.Ping(pctx)
				cancel()
				if perr != nil {
					log.Printf("processamento %s: o modelo caiu: %v", in.ID, perr)
					update(func(r *Run) { r.Done++ })
					if err := flush(); err != nil {
						log.Printf("processamento %s: falha ao gravar as caixas: %v", in.ID, err)
					}
					finish(StatusFailed, "o modelo parou de responder")
					return
				}
				failures = 0
			}
		default:
			failures = 0
			set.Task, set.Source = in.Task, source
			pending = append(pending, *set)
		}
		if ctx.Err() != nil {
			break
		}
		update(func(r *Run) { r.Done++ })
		sinceSave++
		if sinceSave >= batchSize {
			if err := flush(); err != nil {
				log.Printf("processamento %s: falha ao gravar as caixas: %v", in.ID, err)
				finish(StatusFailed, "erro ao gravar as caixas")
				return
			}
		}
	}
	if err := flush(); err != nil {
		log.Printf("processamento %s: falha ao gravar as caixas: %v", in.ID, err)
		finish(StatusFailed, "erro ao gravar as caixas")
		return
	}
	if ctx.Err() != nil {
		finish(StatusCanceled, "")
		return
	}
	finish(StatusDone, "")
}

// list: as imagens dos pacotes no escopo, uma vez cada (a fila pode mudar
// de ordem enquanto lista).
func (s *Service) list(ctx context.Context, run Run) ([]images.Image, error) {
	f := images.ImageFilter{Packages: run.Packages, Limit: listPage}
	switch run.Scope {
	case ScopePending:
		f.Status = images.StatusPending
	case ScopeVerified:
		f.Status = images.StatusVerified
	}
	seen := map[string]bool{}
	var out []images.Image
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, _, err := s.images.List(ctx, f)
		if err != nil {
			return nil, err
		}
		for _, img := range page {
			if !seen[img.SHA1] {
				seen[img.SHA1] = true
				out = append(out, images.Image{SHA1: img.SHA1, Package: img.Package, Path: img.Path})
			}
		}
		if int64(len(page)) < f.Limit {
			return out, nil
		}
		f.Skip += f.Limit
	}
}

// detect lê o arquivo (só lê: ele nunca muda), pega o tamanho e passa pelo
// modelo.
func (s *Service) detect(ctx context.Context, m Model, img images.Image, opt Options) (*boxes.Set, error) {
	data, err := os.ReadFile(s.files.Abs(img.Path))
	if err != nil {
		log.Printf("processamento: ler %s: %v", img.Path, err)
		return nil, errors.New("arquivo não encontrado no disco")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("não foi possível ler o tamanho da imagem")
	}
	found, err := m.Detect(ctx, data, opt)
	if err != nil {
		log.Printf("processamento: modelo em %s: %v", img.SHA1, err)
		return nil, errors.New("o modelo não conseguiu processar a imagem")
	}
	return &boxes.Set{ImageID: img.SHA1, Package: img.Package, Width: cfg.Width, Height: cfg.Height, Boxes: found}, nil
}

// newID: a data (para ordenar e ler) e um pedaço aleatório.
func newID(now time.Time) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return now.Format("20060102-150405") + "-" + hex.EncodeToString(b)
}
