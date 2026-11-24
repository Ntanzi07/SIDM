package processing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/axle"
	"SIDM/internal/boxes"
	"SIDM/internal/catalog"
	"SIDM/internal/images"
	"SIDM/internal/lpr"
	"SIDM/internal/platform"
)

// ---------- os falsos ----------

type fakeImages struct{ list []images.Image }

func (f *fakeImages) List(ctx context.Context, flt images.ImageFilter) ([]images.Image, int64, error) {
	var out []images.Image
	for _, img := range f.list {
		if len(flt.Packages) > 0 && !slices.Contains(flt.Packages, img.Package) {
			continue
		}
		if flt.Status != "" && img.Status != flt.Status {
			continue
		}
		out = append(out, img)
	}
	total := int64(len(out))
	if flt.Skip >= total {
		return nil, total, nil
	}
	out = out[flt.Skip:]
	if flt.Limit > 0 && int64(len(out)) > flt.Limit {
		out = out[:flt.Limit]
	}
	return out, total, nil
}

type fakePackages map[string]bool

func (f fakePackages) FindPackage(ctx context.Context, name string) (*catalog.Package, error) {
	if !f[name] {
		return nil, platform.ErrNotFound
	}
	return &catalog.Package{Name: name}, nil
}

type dirFiles string

func (d dirFiles) Abs(p string) string { return filepath.Join(string(d), p) }

type emptyProfiles struct{}

func (emptyProfiles) ProfileOf(context.Context, string) (*annotation.Profile, error) {
	return &annotation.Profile{Name: "vazio"}, nil
}

// fakeModel acha uma caixa "car" por imagem. block segura cada imagem até
// receber um sinal; failAll dá erro em todas; down derruba o Ping.
type fakeModel struct {
	mu      sync.Mutex
	block   chan struct{}
	failAll bool
	down    bool
	calls   int
	scores  []float64
}

func (m *fakeModel) Info() ModelInfo {
	return ModelInfo{Key: "fake", Label: "Falso", DefaultTask: "pre-fake", Labels: []string{"car"}, DefaultMinScore: 0.5}
}

func (m *fakeModel) Ping(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.down {
		return errors.New("fora do ar")
	}
	return nil
}

func (m *fakeModel) Detect(ctx context.Context, img []byte, opt Options) ([]boxes.Box, error) {
	m.mu.Lock()
	m.calls++
	m.scores = append(m.scores, opt.MinScore)
	block, failAll := m.block, m.failAll
	m.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if failAll {
		return nil, errors.New("caiu")
	}
	return []boxes.Box{{Label: "car", X: 1, Y: 2, W: 3, H: 4}}, nil
}

// writeJPEG grava uma imagem sintética w×h em dir/name.
func writeJPEG(t *testing.T, dir, name string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type env struct {
	s     *Service
	repo  *Memory
	bx    *boxes.Service
	boxes *boxes.Memory
	model *fakeModel
	imgs  *fakeImages
}

// newEnv: o pacote p1 com a1, a2 (pendentes), a3 (pendente, sem arquivo) e
// v1 (verificada); o pacote p2 com b1 (pendente).
func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	imgs := &fakeImages{}
	add := func(sha, pkg, status string, w, h int, file bool) {
		path := "dataset/" + pkg + "/" + sha + ".jpg"
		if file {
			writeJPEG(t, dir, path, w, h)
		}
		imgs.list = append(imgs.list, images.Image{SHA1: sha, Package: pkg, Status: status, Path: path})
	}
	add("a1", "p1", images.StatusPending, 64, 48, true)
	add("a2", "p1", images.StatusPending, 32, 16, true)
	add("a3", "p1", images.StatusPending, 0, 0, false)
	add("v1", "p1", images.StatusVerified, 20, 10, true)
	add("b1", "p2", images.StatusPending, 20, 10, true)

	bm := boxes.NewMemory()
	bx := boxes.New(bm, nil, nil, nil, emptyProfiles{})
	repo := NewMemory()
	model := &fakeModel{}
	s := New(repo, imgs, fakePackages{"p1": true, "p2": true}, dirFiles(dir), bx, model)
	return &env{s: s, repo: repo, bx: bx, boxes: bm, model: model, imgs: imgs}
}

func (e *env) run(t *testing.T, in RunInput) *Run {
	t.Helper()
	run, err := e.s.Start(context.Background(), in, "ana")
	if err != nil {
		t.Fatal(err)
	}
	e.s.Wait()
	got, err := e.repo.FindRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// ---------- os testes ----------

func TestProcessaPendentes(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	// a2 já tem caixas do CVAT na tarefa: fica como está
	e.boxes.Put(boxes.Set{Task: "pre-fake", ImageID: "a2", Package: "p1", Source: boxes.Source{Tool: "cvat"}})

	run := e.run(t, RunInput{Model: "fake", Packages: []string{"p1"}})
	if run.Status != StatusDone || run.Scope != ScopePending || run.Task != "pre-fake" || run.MinScore != 0.5 || run.StartedBy != "ana" || run.FinishedAt == nil {
		t.Fatalf("run: %+v", run)
	}
	if run.Total != 3 || run.Done != 3 || run.Written != 1 || run.Boxes != 1 || run.Skipped != 1 || run.ErrorCount != 1 {
		t.Errorf("contagens: %+v", run)
	}
	if len(run.Errors) != 1 || run.Errors[0].SHA1 != "a3" || len(run.SkippedIDs) != 1 || run.SkippedIDs[0] != "a2" {
		t.Errorf("relatório: errors %+v, skipped %v", run.Errors, run.SkippedIDs)
	}

	task, err := e.bx.FindTask(ctx, "pre-fake")
	if err != nil || !slices.Contains(task.Labels, "car") {
		t.Fatalf("tarefa: %+v %v", task, err)
	}
	sets, _ := e.bx.FindSets(ctx, "pre-fake", []string{"a1", "a2", "v1", "b1"})
	a1 := sets["a1"]
	if a1.Width != 64 || a1.Height != 48 || len(a1.Boxes) != 1 || a1.Source.Tool != "fake" || a1.Source.File != "run:"+run.ID || a1.ImportedBy != "ana" {
		t.Errorf("caixas de a1: %+v", a1)
	}
	if sets["a2"].Source.Tool != "cvat" {
		t.Error("as caixas do CVAT mudaram")
	}
	if _, ok := sets["v1"]; ok {
		t.Error("verificada entrou no escopo pendentes")
	}
	if _, ok := sets["b1"]; ok {
		t.Error("pacote não pedido entrou")
	}
}

func TestEscopos(t *testing.T) {
	e := newEnv(t)
	if run := e.run(t, RunInput{Model: "fake", Packages: []string{"p1"}, Scope: ScopeVerified}); run.Total != 1 || run.Written != 1 {
		t.Errorf("só verificadas: %+v", run)
	}
	min := 0.8
	run := e.run(t, RunInput{Model: "fake", Packages: []string{"p1", "p2", "p1"}, Scope: ScopeAll, Task: "outra", MinScore: &min})
	if run.Total != 5 || run.Task != "outra" || strings.Join(run.Packages, ",") != "p1,p2" {
		t.Errorf("todas: %+v", run)
	}
	if last := e.model.scores[len(e.model.scores)-1]; last != 0.8 {
		t.Errorf("o score mínimo chega ao modelo: %v", last)
	}
	// rodar de novo é seguro: regrava só o que é do próprio modelo
	if run := e.run(t, RunInput{Model: "fake", Packages: []string{"p1"}, Scope: ScopeVerified}); run.Written != 1 || run.Skipped != 0 {
		t.Errorf("de novo: %+v", run)
	}
}

func TestValidacao(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	bad, big := -0.1, 1.5
	var invalid *InvalidError
	var boxInvalid *boxes.InvalidError
	for _, in := range []RunInput{
		{Model: "nenhum", Packages: []string{"p1"}},
		{Model: "fake"},
		{Model: "fake", Packages: []string{" ", ""}},
		{Model: "fake", Packages: []string{"nao-existe"}},
		{Model: "fake", Packages: []string{"p1"}, Scope: "metade"},
		{Model: "fake", Packages: []string{"p1"}, MinScore: &bad},
		{Model: "fake", Packages: []string{"p1"}, MinScore: &big},
	} {
		if _, err := e.s.Start(ctx, in, "ana"); !errors.As(err, &invalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	if _, err := e.s.Start(ctx, RunInput{Model: "fake", Packages: []string{"p1"}, Task: "Nome Ruim"}, "ana"); !errors.As(err, &boxInvalid) {
		t.Errorf("tarefa inválida: %v", err)
	}
	e.model.down = true
	var unavailable *UnavailableError
	if _, err := e.s.Start(ctx, RunInput{Model: "fake", Packages: []string{"p1"}}, "ana"); !errors.As(err, &unavailable) {
		t.Errorf("modelo fora do ar: %v", err)
	}
	if st := e.s.Models(ctx); len(st) != 1 || st[0].Available || st[0].Error == "" || strings.Contains(st[0].Error, "fora do ar") {
		t.Errorf("models (sem o detalhe do erro): %+v", st)
	}
	if len(e.repo.Runs) != 0 {
		t.Errorf("pedido recusado não grava: %d", len(e.repo.Runs))
	}
}

func TestOcupadoECancelar(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.model.block = make(chan struct{})

	run, err := e.s.Start(ctx, RunInput{Model: "fake", Packages: []string{"p1"}, Scope: ScopeAll}, "ana")
	if err != nil {
		t.Fatal(err)
	}
	var conflict *ConflictError
	if _, err := e.s.Start(ctx, RunInput{Model: "fake", Packages: []string{"p2"}}, "bia"); !errors.As(err, &conflict) {
		t.Errorf("modelo ocupado: %v", err)
	}
	if st := e.s.Models(ctx); !st[0].Busy {
		t.Error("o modelo deveria aparecer ocupado")
	}
	e.model.block <- struct{}{} // a primeira imagem passa
	waitFor(t, func() bool { r, _ := e.s.FindRun(ctx, run.ID); return r.Done >= 1 })

	if got, err := e.s.Cancel(ctx, run.ID); err != nil || got.ID != run.ID {
		t.Fatalf("cancelar: %+v %v", got, err)
	}
	e.s.Wait()
	got, _ := e.s.FindRun(ctx, run.ID)
	if got.Status != StatusCanceled || got.Done != 1 || got.Written != 1 || got.FinishedAt == nil {
		t.Errorf("cancelado: %+v", got)
	}
	if _, err := e.s.Cancel(ctx, run.ID); !errors.As(err, &conflict) {
		t.Errorf("cancelar de novo: %v", err)
	}
	if _, err := e.s.Cancel(ctx, "nao-existe"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("cancelar o que não existe: %v", err)
	}
	// livre de novo
	e.model.block = nil
	if run := e.run(t, RunInput{Model: "fake", Packages: []string{"p2"}}); run.Status != StatusDone {
		t.Errorf("depois de cancelar: %+v", run)
	}
}

func TestModeloCai(t *testing.T) {
	e := newEnv(t)
	for i := range 10 { // imagens suficientes para passar de maxFailures
		e.imgs.list = append(e.imgs.list, e.imgs.list[0])
		e.imgs.list[len(e.imgs.list)-1].SHA1 = "x" + string(rune('a'+i))
	}
	e.model.failAll = true
	run, err := e.s.Start(context.Background(), RunInput{Model: "fake", Packages: []string{"p1"}}, "ana")
	if err != nil {
		t.Fatal(err)
	}
	e.model.mu.Lock()
	e.model.down = true
	e.model.mu.Unlock()
	e.s.Wait()
	got, _ := e.repo.FindRun(context.Background(), run.ID)
	if got.Status != StatusFailed || got.Message == "" || got.ErrorCount != maxFailures || got.Done != maxFailures {
		t.Errorf("modelo caiu: %+v", got)
	}
	if strings.Contains(got.Errors[0].Msg, "caiu") {
		t.Errorf("o detalhe do erro vai para o log, não para o relatório: %q", got.Errors[0].Msg)
	}
}

func TestMarkInterrupted(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.repo.Runs["r1"] = Run{ID: "r1", Status: StatusRunning}
	e.repo.Runs["r2"] = Run{ID: "r2", Status: StatusDone}
	if n, err := e.s.MarkInterrupted(ctx); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if e.repo.Runs["r1"].Status != StatusInterrupted || e.repo.Runs["r1"].FinishedAt == nil || e.repo.Runs["r2"].Status != StatusDone {
		t.Errorf("%+v", e.repo.Runs)
	}
	runs, _ := e.s.ListRuns(ctx, 50)
	if len(runs) != 2 {
		t.Errorf("lista: %+v", runs)
	}
}

func TestLPRBoxes(t *testing.T) {
	o := &lpr.Objects{
		Vehicle: []lpr.Box{
			{TopLeft: lpr.Point{X: 0, Y: 0}, BottomRight: lpr.Point{X: 100, Y: 80}, Type: lpr.VehicleMotorcycle},
			{TopLeft: lpr.Point{X: 5, Y: 5}, BottomRight: lpr.Point{X: 3, Y: 9}, Type: 9},
		},
		Plate: []lpr.Box{{TopLeft: lpr.Point{X: 10, Y: 20}, BottomRight: lpr.Point{X: 60, Y: 40}}, {}},
		Character: []lpr.CharacterBox{
			{TopLeft: lpr.Point{X: 30, Y: 22}, BottomRight: lpr.Point{X: 35, Y: 30}, Char: "B", Score: 0.91234},
			{TopLeft: lpr.Point{X: 12, Y: 22}, BottomRight: lpr.Point{X: 17, Y: 30}, Char: "A", Score: 0.8},
			{TopLeft: lpr.Point{X: 40, Y: 22}, Char: "X", Score: 0.2},
		},
	}
	got := LPRBoxes(o, 0.5)
	labels := []string{}
	for _, b := range got {
		labels = append(labels, b.Label)
	}
	if strings.Join(labels, ",") != "motorcycle,vehicle,plate,plate,B,A" {
		t.Fatalf("labels: %v", labels)
	}
	if got[0].W != 100 || got[0].H != 80 || got[1].W != 0 {
		t.Errorf("tamanhos: %+v %+v", got[0], got[1])
	}
	if got[2].Attributes["text"] != "AB" || got[3].Attributes != nil {
		t.Errorf("texto das placas: %v / %v", got[2].Attributes, got[3].Attributes)
	}
	if got[4].Attributes["score"] != 0.912 || got[4].X != 30 || got[4].W != 5 {
		t.Errorf("caractere: %+v", got[4])
	}
	if n := len(LPRLabels()); n != 5+10+26 {
		t.Errorf("labels da tarefa: %d", n)
	}
}

func TestHTTP(t *testing.T) {
	e := newEnv(t)
	mux := http.NewServeMux()
	for _, rt := range e.s.Routes() {
		mux.HandleFunc(rt.Method+" "+rt.Path, rt.Handler)
	}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-User", "ana")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := do("GET", "/processing/models", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"available":true`) {
		t.Errorf("models: %d %s", rec.Code, rec.Body)
	}
	if rec := do("POST", "/processing/runs", `{"model":"fake"}`); rec.Code != 400 {
		t.Errorf("sem pacote: %d", rec.Code)
	}
	if rec := do("POST", "/processing/runs", `{`); rec.Code != 400 {
		t.Errorf("JSON ruim: %d", rec.Code)
	}
	rec := do("POST", "/processing/runs", `{"model":"fake","packages":["p2"]}`)
	if rec.Code != 202 {
		t.Fatalf("iniciar: %d %s", rec.Code, rec.Body)
	}
	e.s.Wait()
	if rec := do("GET", "/processing/runs", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"done"`) {
		t.Errorf("lista: %d %s", rec.Code, rec.Body)
	}
	if rec := do("GET", "/processing/runs/nao-existe", ""); rec.Code != 404 {
		t.Errorf("não existe: %d", rec.Code)
	}
	e.model.down = true
	if rec := do("POST", "/processing/runs", `{"model":"fake","packages":["p2"]}`); rec.Code != 503 {
		t.Errorf("fora do ar: %d", rec.Code)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("tempo esgotado")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAxleBoxes(t *testing.T) {
	r := &axle.Result{Detections: []axle.Detection{
		{ClassID: 0, ClassName: "axle", Score: 0.91234, BboxX1: 10, BboxY1: 20, BboxX2: 40, BboxY2: 60},
		{ClassID: 9, ClassName: "unknown", Score: 0.6, BboxX1: 5, BboxY1: 5, BboxX2: 3, BboxY2: 9},
		{ClassID: 2, ClassName: "class_b", Score: 0.3},
	}}
	got := AxleBoxes(r, 0.5)
	if len(got) != 2 || got[0].Label != "axle" || got[1].Label != "class_9" {
		t.Fatalf("caixas: %+v", got)
	}
	if got[0].X != 10 || got[0].Y != 20 || got[0].W != 30 || got[0].H != 40 || got[0].Attributes["score"] != 0.912 || got[1].W != 0 {
		t.Errorf("tamanhos/score: %+v", got)
	}
	if n := len(AxleBoxes(r, 0)); n != 3 {
		t.Errorf("sem corte: %d", n)
	}
	if info := NewAxle(nil).Info(); info.DefaultMinScore != 0 || info.DefaultTask != "pre-eixos" || len(info.Labels) != 9 {
		t.Errorf("info: %+v", info)
	}
}
