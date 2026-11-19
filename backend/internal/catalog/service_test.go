package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// fakeDirs grava o que foi pedido ao disco.
type fakeDirs struct {
	calls     []string
	renameErr error
	removeErr error
}

func (d *fakeDirs) EnsurePackageDir(name string) error {
	d.calls = append(d.calls, "ensure "+name)
	return nil
}

func (d *fakeDirs) RenamePackageDir(oldName, newName string) error {
	d.calls = append(d.calls, "rename "+oldName+"->"+newName)
	return d.renameErr
}

func (d *fakeDirs) RemovePackageDir(name string) error {
	d.calls = append(d.calls, "remove "+name)
	return d.removeErr
}

// fakeImages faz o papel do módulo de imagens; log guarda a ordem das chamadas.
type fakeImages struct {
	log      *[]string
	progress map[string]Progress
	using    int64
	failed   []string
}

func (f *fakeImages) PackageProgress(ctx context.Context, names []string) (map[string]Progress, error) {
	return f.progress, nil
}

func (f *fakeImages) RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error) {
	*f.log = append(*f.log, "images rename")
	return 7, nil
}

func (f *fakeImages) DeletePackageImages(ctx context.Context, name string) (int64, error) {
	*f.log = append(*f.log, "images delete")
	return 3, nil
}

func (f *fakeImages) CountUsing(ctx context.Context, reg Registry, name string) (int64, error) {
	return f.using, nil
}

func (f *fakeImages) RenameRegistry(ctx context.Context, reg Registry, oldName, newName, user string) (RenameResult, error) {
	*f.log = append(*f.log, fmt.Sprintf("images %s %s->%s by %s", reg, oldName, newName, user))
	return RenameResult{PendingUpdated: 2, VerifiedRenamed: 1, Failed: f.failed}, nil
}

type fakeListener struct{ log *[]string }

func (l fakeListener) PackageRenamed(ctx context.Context, oldName, newName string) error {
	*l.log = append(*l.log, "listener rename")
	return nil
}

func (l fakeListener) PackageDeleted(ctx context.Context, name string) error {
	*l.log = append(*l.log, "listener delete")
	return nil
}

func newTestService() (*Service, *Memory, *fakeDirs, *fakeImages, *[]string) {
	var log []string
	mem, dirs := NewMemory(), &fakeDirs{}
	imgs := &fakeImages{log: &log}
	return New(mem, dirs, imgs, nil, fakeListener{log: &log}), mem, dirs, imgs, &log
}

func TestServicePacotes(t *testing.T) {
	s, mem, dirs, imgs, log := newTestService()
	ctx := context.Background()

	if err := s.CreatePackage(ctx, "p1", "", "nathan"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePackage(ctx, "p1", "", "nathan"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("duplicado: %v", err)
	}
	if fmt.Sprint(dirs.calls) != "[ensure p1]" || mem.Packages["p1"].CreatedBy != "nathan" || mem.Packages["p1"].CreatedAt.IsZero() {
		t.Errorf("criar: dirs=%v pacote=%+v", dirs.calls, mem.Packages["p1"])
	}

	s.CreatePackage(ctx, "p2", "", "nathan")
	imgs.progress = map[string]Progress{"p1": {Total: 3, Verified: 2, Pending: 1}}
	list, total, err := s.ListPackages(ctx, "", 10, 0)
	if err != nil || total != 2 || list[0].Total != 3 || list[1].Total != 0 {
		t.Errorf("ListPackages = %+v, %d, %v", list, total, err)
	}

	// renomear: pasta, cadastro, imagens, avisos
	dirs.calls = nil
	n, err := s.RenamePackage(ctx, "p1", "novo")
	if err != nil || n != 7 {
		t.Fatalf("RenamePackage = %d, %v", n, err)
	}
	if fmt.Sprint(dirs.calls) != "[rename p1->novo]" || strings.Join(*log, ",") != "images rename,listener rename" {
		t.Errorf("ordem: dirs=%v log=%v", dirs.calls, *log)
	}
	if _, ok := mem.Packages["novo"]; !ok {
		t.Error("o cadastro deveria ter o nome novo")
	}
	if _, err := s.RenamePackage(ctx, "novo", "p2"); !errors.Is(err, platform.ErrDuplicate) {
		t.Errorf("para nome existente: %v", err)
	}
	if _, err := s.RenamePackage(ctx, "sumiu", "x"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	if n, err := s.RenamePackage(ctx, "novo", "novo"); err != nil || n != 0 {
		t.Errorf("mesmo nome: %d, %v", n, err)
	}

	// a pasta não muda: nada no banco muda
	dirs.calls, *log, dirs.renameErr = nil, nil, errors.New("disco cheio")
	if _, err := s.RenamePackage(ctx, "novo", "outro"); err == nil {
		t.Error("deveria falhar")
	}
	if _, ok := mem.Packages["novo"]; !ok || len(*log) != 0 {
		t.Errorf("com a pasta falhando, nada pode mudar: %v", *log)
	}
	dirs.renameErr = nil

	// apagar: avisos, imagens, cadastro, pasta
	dirs.calls, *log = nil, nil
	res, err := s.DeletePackage(ctx, "novo")
	if err != nil || res.Images != 3 || len(res.Warnings) != 0 {
		t.Fatalf("DeletePackage = %+v, %v", res, err)
	}
	if strings.Join(*log, ",") != "listener delete,images delete" || fmt.Sprint(dirs.calls) != "[remove novo]" {
		t.Errorf("ordem: log=%v dirs=%v", *log, dirs.calls)
	}
	if _, ok := mem.Packages["novo"]; ok {
		t.Error("o pacote deveria ter sumido")
	}
	if _, err := s.DeletePackage(ctx, "novo"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("apagar duas vezes: %v", err)
	}
	// a pasta não sai: os registros já foram, e vem um aviso
	dirs.removeErr = errors.New("em uso")
	if res, err := s.DeletePackage(ctx, "p2"); err != nil || len(res.Warnings) != 1 {
		t.Errorf("pasta presa: %+v, %v", res, err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestServiceCadastro(t *testing.T) {
	s, mem, _, imgs, log := newTestService()
	ctx := context.Background()
	mem.Equipments["velho"] = true

	// renomear para um nome novo: cria o novo, avisa as imagens, apaga o velho
	rep, err := s.Update(ctx, Equipments, "velho", ptr("novo"), nil, "nathan")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Renamed || rep.Merged || rep.Name != "novo" || !rep.Complete || rep.PendingUpdated != 2 || rep.VerifiedRenamed != 1 {
		t.Errorf("rename = %+v", rep)
	}
	if _, ok := mem.Equipments["velho"]; ok || !mem.Equipments["novo"] {
		t.Errorf("cadastro = %v", mem.Equipments)
	}
	if fmt.Sprint(*log) != "[images equipments velho->novo by nathan]" {
		t.Errorf("imagens: %v", *log)
	}

	// renomear para um que já existe: FUNDE (fica ativo se algum dos dois era)
	mem.Equipments["a"], mem.Equipments["b"] = true, false
	rep, _ = s.Update(ctx, Equipments, "a", ptr("b"), nil, "nathan")
	if !rep.Merged || !rep.Active || !mem.Equipments["b"] {
		t.Errorf("fundir = %+v %v", rep, mem.Equipments)
	}

	// alguma verificada falhou: o nome antigo fica, para repetir depois
	mem.Equipments["c"] = true
	imgs.failed = []string{"sha-x"}
	rep, _ = s.Update(ctx, Equipments, "c", ptr("d"), nil, "nathan")
	if rep.Complete || fmt.Sprint(rep.Failed) != "[sha-x]" {
		t.Errorf("com falha = %+v", rep)
	}
	if _, ok := mem.Equipments["c"]; !ok {
		t.Error("com falha, o nome antigo tem que ficar")
	}
	imgs.failed = nil

	// só desativar
	rep, _ = s.Update(ctx, Equipments, "d", nil, ptr(false), "nathan")
	if rep.Renamed || rep.Active || mem.Equipments["d"] {
		t.Errorf("desativar = %+v", rep)
	}
	if _, err := s.Update(ctx, Equipments, "sumiu", nil, ptr(true), "nathan"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("inexistente: %v", err)
	}

	// remover: em uso desativa, sem uso apaga
	mem.Cameras["cam"] = true
	imgs.using = 2
	if res, _ := s.Remove(ctx, Cameras, "cam"); !res.Deactivated || res.Deleted || mem.Cameras["cam"] {
		t.Errorf("em uso = %+v", res)
	}
	imgs.using = 0
	if res, _ := s.Remove(ctx, Cameras, "cam"); !res.Deleted {
		t.Errorf("sem uso = %+v", res)
	}
	if _, ok := mem.Cameras["cam"]; ok {
		t.Error("deveria ter sido apagada")
	}
}

// fakeProfiles: só o "capacete" existe além do padrão.
type fakeProfiles struct{}

func (fakeProfiles) FindProfile(ctx context.Context, name string) (*annotation.Profile, error) {
	if name == "capacete" {
		return &annotation.Profile{Name: name}, nil
	}
	return nil, platform.ErrNotFound
}

func TestPerfilDoPacote(t *testing.T) {
	s, mem, _, imgs, _ := newTestService()
	s.profiles = fakeProfiles{}
	ctx := context.Background()

	// sem perfil: o padrão
	if err := s.CreatePackage(ctx, "p1", "", "nathan"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.FindPackage(ctx, "p1"); p.Profile != annotation.DefaultProfile {
		t.Errorf("perfil = %q", p.Profile)
	}
	// perfil que não existe
	var invalid *InvalidError
	if err := s.CreatePackage(ctx, "p2", "nao-existe", "nathan"); !errors.As(err, &invalid) {
		t.Errorf("perfil inexistente: %v", err)
	}
	if _, ok := mem.Packages["p2"]; ok {
		t.Error("criou o pacote com perfil inexistente")
	}
	if err := s.CreatePackage(ctx, "p3", "capacete", "nathan"); err != nil || mem.Packages["p3"].Profile != "capacete" {
		t.Errorf("criar com perfil: %v, %+v", err, mem.Packages["p3"])
	}

	// pacote de antes dos perfis (vazio no banco) aparece com o padrão
	mem.Packages["antigo"] = Package{Name: "antigo"}
	if p, _ := s.FindPackage(ctx, "antigo"); p.Profile != annotation.DefaultProfile {
		t.Errorf("antigo: %q", p.Profile)
	}

	// trocar: livre sem verificadas, recusado com
	if err := s.SetPackageProfile(ctx, "p1", "capacete"); err != nil || mem.Packages["p1"].Profile != "capacete" {
		t.Errorf("trocar: %v", err)
	}
	imgs.progress = map[string]Progress{"p1": {Verified: 3, Total: 3}}
	if err := s.SetPackageProfile(ctx, "p1", annotation.DefaultProfile); !errors.As(err, &invalid) || mem.Packages["p1"].Profile != "capacete" {
		t.Errorf("trocar com verificadas: %v", err)
	}
	if err := s.SetPackageProfile(ctx, "nao-existe", "capacete"); !errors.Is(err, platform.ErrNotFound) {
		t.Errorf("pacote inexistente: %v", err)
	}
}

// Os pacotes de antes dos perfis (sem o campo) contam como o padrão: a
// pesquisa de "veiculos" precisa achar as imagens deles.
func TestPacotesDoPerfilPadrao(t *testing.T) {
	s, mem, _, _, _ := newTestService()
	ctx := context.Background()
	mem.Packages["antigo"] = Package{Name: "antigo"}
	mem.Packages["novo"] = Package{Name: "novo", Profile: annotation.DefaultProfile}
	mem.Packages["moto"] = Package{Name: "moto", Profile: "capacete"}
	if got, err := s.PackagesWithProfile(ctx, annotation.DefaultProfile); err != nil || strings.Join(got, ",") != "antigo,novo" {
		t.Errorf("padrão = %v, %v", got, err)
	}
	if got, _ := s.PackagesWithProfile(ctx, "capacete"); strings.Join(got, ",") != "moto" {
		t.Errorf("capacete = %v", got)
	}
}
