package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"SIDM/internal/annotation"
	"SIDM/internal/platform"
)

// Service são as regras do catálogo. Outros módulos usam o Service (e não o
// Repo) para consultar pacotes e cadastros.
type Service struct {
	repo      Repo
	dirs      Dirs
	images    Images
	profiles  Profiles
	listeners []PackageListener
	now       func() time.Time
}

// New monta o catálogo. images pode ser nil só em testes que não mexem em
// imagens; profiles nil aceita só o perfil padrão.
func New(repo Repo, dirs Dirs, images Images, profiles Profiles, listeners ...PackageListener) *Service {
	return &Service{repo: repo, dirs: dirs, images: images, profiles: profiles, listeners: listeners,
		now: func() time.Time { return time.Now().UTC() }}
}

// Listen acrescenta quem quer ser avisado das mudanças de pacote. Existe
// porque o aviso às vezes vai para um módulo que depende do catálogo (as
// caixas pedem o pacote da URL a ele): o main cria os dois e depois liga.
func (s *Service) Listen(l PackageListener) { s.listeners = append(s.listeners, l) }

// ---------- pacotes ----------

// CreatePackage cadastra o pacote (nome já sanitizado) e cria a pasta: um
// pacote vazio também aparece no disco.
// profile vazio = o perfil padrão.
func (s *Service) CreatePackage(ctx context.Context, name, profile, user string) error {
	profile, err := s.checkProfile(ctx, profile)
	if err != nil {
		return err
	}
	if err := s.repo.CreatePackage(ctx, Package{Name: name, Profile: profile, CreatedBy: user, CreatedAt: s.now()}); err != nil {
		return err
	}
	return s.dirs.EnsurePackageDir(name)
}

// checkProfile confere que o perfil de anotação existe (vazio = o padrão).
func (s *Service) checkProfile(ctx context.Context, profile string) (string, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" || profile == annotation.DefaultProfile {
		return annotation.DefaultProfile, nil
	}
	if s.profiles == nil {
		return "", &InvalidError{fmt.Sprintf("perfil de anotação %q não existe", profile)}
	}
	if _, err := s.profiles.FindProfile(ctx, profile); errors.Is(err, platform.ErrNotFound) {
		return "", &InvalidError{fmt.Sprintf("perfil de anotação %q não existe", profile)}
	} else if err != nil {
		return "", err
	}
	return profile, nil
}

// FindPackage busca um pacote (platform.ErrNotFound se não existe).
func (s *Service) FindPackage(ctx context.Context, name string) (*Package, error) {
	p, err := s.repo.FindPackage(ctx, name)
	if err != nil {
		return nil, err
	}
	p.normalize()
	return p, nil
}

// normalize: pacote de antes dos perfis usa o padrão.
func (p *Package) normalize() {
	if p.Profile == "" {
		p.Profile = annotation.DefaultProfile
	}
}

// SetPackageProfile troca o perfil de anotação do pacote. Só enquanto não
// há imagem verificada: o nome do arquivo de uma verificada saiu dos campos
// do perfil antigo. (As pendentes ficam com o rascunho; o que o perfil novo
// não tiver é recusado ao salvar, e a pessoa corrige.)
func (s *Service) SetPackageProfile(ctx context.Context, name, profile string) error {
	if _, err := s.repo.FindPackage(ctx, name); err != nil {
		return err
	}
	profile, err := s.checkProfile(ctx, profile)
	if err != nil {
		return err
	}
	progress, err := s.images.PackageProgress(ctx, []string{name})
	if err != nil {
		return err
	}
	if n := progress[name].Verified; n > 0 {
		return &InvalidError{fmt.Sprintf("o pacote já tem %d imagens verificadas com o perfil atual: o perfil não pode mais ser trocado", n)}
	}
	return s.repo.SetPackageProfile(ctx, name, profile)
}

// ListPackages devolve uma página de pacotes com o progresso de cada um.
func (s *Service) ListPackages(ctx context.Context, q string, limit, skip int64) ([]PackageSummary, int64, error) {
	pkgs, total, err := s.repo.ListPackages(ctx, q, limit, skip)
	if err != nil {
		return nil, 0, err
	}
	out := make([]PackageSummary, 0, len(pkgs))
	if len(pkgs) == 0 {
		return out, total, nil
	}
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	progress, err := s.images.PackageProgress(ctx, names)
	if err != nil {
		return nil, 0, err
	}
	for _, p := range pkgs {
		p.normalize()
		out = append(out, PackageSummary{Package: p, Progress: progress[p.Name]})
	}
	return out, total, nil
}

// RenamePackage renomeia a pasta, o cadastro e o caminho de todas as imagens.
// O nome dos arquivos não muda (o pacote não faz parte do nome v2).
//
// Ordem: a pasta primeiro (um rename, instantâneo) e o cadastro, voltando a
// pasta se o cadastro falhar; depois as imagens e os avisos.
func (s *Service) RenamePackage(ctx context.Context, oldName, newName string) (int64, error) {
	if _, err := s.repo.FindPackage(ctx, oldName); err != nil {
		return 0, err
	}
	if oldName == newName {
		return 0, nil
	}
	if _, err := s.repo.FindPackage(ctx, newName); err == nil {
		return 0, platform.ErrDuplicate
	} else if !errors.Is(err, platform.ErrNotFound) {
		return 0, err
	}
	if err := s.dirs.RenamePackageDir(oldName, newName); err != nil {
		return 0, err
	}
	if err := s.repo.RenamePackage(ctx, oldName, newName); err != nil {
		_ = s.dirs.RenamePackageDir(newName, oldName)
		return 0, err
	}
	n, err := s.images.RenamePackageImages(ctx, oldName, newName)
	if err != nil {
		return n, fmt.Errorf("renomear o pacote nas imagens: %w", err)
	}
	for _, l := range s.listeners {
		if err := l.PackageRenamed(ctx, oldName, newName); err != nil {
			return n, err
		}
	}
	return n, nil
}

// DeleteResult é o que sobrou de apagar um pacote.
type DeleteResult struct {
	Images   int64
	Warnings []string
}

// DeletePackage apaga o pacote, TODAS as imagens dele (banco e arquivos), o
// histórico e o que os outros módulos guardam dele.
//
// Banco primeiro, e o pacote por último: se algo falhar no meio, ele
// continua existindo e dá para repetir. A pasta só depois: se o disco
// falhar, sobram arquivos órfãos (avisados), mas nunca um registro apontando
// para um arquivo que não existe.
func (s *Service) DeletePackage(ctx context.Context, name string) (*DeleteResult, error) {
	if _, err := s.repo.FindPackage(ctx, name); err != nil {
		return nil, err
	}
	for _, l := range s.listeners {
		if err := l.PackageDeleted(ctx, name); err != nil {
			return nil, err
		}
	}
	n, err := s.images.DeletePackageImages(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeletePackage(ctx, name); err != nil && !errors.Is(err, platform.ErrNotFound) {
		return nil, err
	}
	res := &DeleteResult{Images: n, Warnings: []string{}}
	if err := s.dirs.RemovePackageDir(name); err != nil {
		platform.LogError(err)
		res.Warnings = append(res.Warnings, "os registros foram apagados, mas a pasta não: apague dataset/"+name+" manualmente")
	}
	return res, nil
}

// ---------- equipamentos e câmeras ----------

// Search busca os itens ativos (autocomplete).
func (s *Service) Search(ctx context.Context, reg Registry, q string, limit, skip int64) ([]string, int64, error) {
	return s.repo.Search(ctx, reg, q, limit, skip)
}

// Create cadastra um item (nome já sanitizado). platform.ErrDuplicate se existe.
func (s *Service) Create(ctx context.Context, reg Registry, name string) error {
	return s.repo.Create(ctx, reg, name)
}

// Find busca um item, ativo ou não.
func (s *Service) Find(ctx context.Context, reg Registry, name string) (*RegistryEntry, error) {
	return s.repo.Find(ctx, reg, name)
}

// ActiveExists: o nome está cadastrado e ativo?
func (s *Service) ActiveExists(ctx context.Context, reg Registry, name string) (bool, error) {
	return s.repo.ActiveExists(ctx, reg, name)
}

// EquipmentExists e CameraExists: atalhos de ActiveExists (o import e a
// anotação recebem funções com essa assinatura).
func (s *Service) EquipmentExists(ctx context.Context, name string) (bool, error) {
	return s.repo.ActiveExists(ctx, Equipments, name)
}

func (s *Service) CameraExists(ctx context.Context, name string) (bool, error) {
	return s.repo.ActiveExists(ctx, Cameras, name)
}

// CreateEquipment e CreateCamera: atalhos de Create.
func (s *Service) CreateEquipment(ctx context.Context, name string) error {
	return s.repo.Create(ctx, Equipments, name)
}

func (s *Service) CreateCamera(ctx context.Context, name string) error {
	return s.repo.Create(ctx, Cameras, name)
}

// UpdateReport é o resultado de PATCH /equipments/{name} (e câmeras).
type UpdateReport struct {
	Name            string   `json:"name"`             // o nome final
	Renamed         bool     `json:"renamed"`          // o nome mudou
	Merged          bool     `json:"merged"`           // o novo nome já existia: os dois viraram um
	Active          bool     `json:"active"`           //
	PendingUpdated  int64    `json:"pending_updated"`  // pendentes que trocaram de nome (só no banco)
	VerifiedRenamed int      `json:"verified_renamed"` // verificadas: arquivo renomeado para o novo nome v2
	Failed          []string `json:"failed"`           // sha1 das que não deu para renomear agora
	Complete        bool     `json:"complete"`         // false: rode de novo para terminar as que falharam
}

// Update renomeia (em cascata) e/ou ativa/desativa um item. newName já vem
// sanitizado; nil = não muda.
//
// A renomeação foi pensada para poder ser REPETIDA se algo falhar no meio:
//
//  1. garante que o nome novo existe no cadastro (se já existia, FUNDE os dois);
//  2. troca o nome nas imagens (pendentes no banco, verificadas no arquivo);
//  3. só então apaga o nome antigo. Se alguma verificada falhou, o antigo
//     fica, e o mesmo pedido pode ser repetido para terminar.
func (s *Service) Update(ctx context.Context, reg Registry, oldName string, newName *string, active *bool, user string) (*UpdateReport, error) {
	old, err := s.repo.Find(ctx, reg, oldName)
	if err != nil {
		return nil, err
	}
	rep := &UpdateReport{Name: oldName, Active: old.Active, Failed: []string{}, Complete: true}

	if newName != nil && *newName != oldName {
		if err := s.rename(ctx, reg, old, *newName, user, rep); err != nil {
			return nil, err
		}
	}
	if active != nil {
		if err := s.repo.SetActive(ctx, reg, rep.Name, *active); err != nil {
			return nil, err
		}
		rep.Active = *active
	}
	return rep, nil
}

func (s *Service) rename(ctx context.Context, reg Registry, old *RegistryEntry, newName, user string, rep *UpdateReport) error {
	// 1. o nome novo
	target, err := s.repo.Find(ctx, reg, newName)
	switch {
	case err == nil:
		rep.Merged = true
		rep.Active = target.Active || old.Active
	case errors.Is(err, platform.ErrNotFound):
		if err := s.repo.Create(ctx, reg, newName); err != nil && !errors.Is(err, platform.ErrDuplicate) {
			return err
		}
		rep.Active = old.Active
	default:
		return err
	}
	if err := s.repo.SetActive(ctx, reg, newName, rep.Active); err != nil {
		return err
	}

	// 2. as imagens
	res, err := s.images.RenameRegistry(ctx, reg, old.Name, newName, user)
	if err != nil {
		return err
	}
	rep.PendingUpdated, rep.VerifiedRenamed = res.PendingUpdated, res.VerifiedRenamed
	if res.Failed != nil {
		rep.Failed = res.Failed
	}

	// 3. o nome antigo, só se tudo deu certo
	rep.Name, rep.Renamed = newName, true
	if len(rep.Failed) > 0 {
		rep.Complete = false
		return nil
	}
	if err := s.repo.Delete(ctx, reg, old.Name); err != nil && !errors.Is(err, platform.ErrNotFound) {
		return err
	}
	return nil
}

// RemoveResult é o resultado de DELETE /equipments/{name} (e câmeras).
type RemoveResult struct {
	Name        string `json:"name"`
	Images      int64  `json:"images"`
	Deleted     bool   `json:"deleted"`
	Deactivated bool   `json:"deactivated"`
}

// Remove: em uso por alguma imagem, DESATIVA (as imagens continuam
// intactas); sem uso, apaga de vez.
func (s *Service) Remove(ctx context.Context, reg Registry, name string) (*RemoveResult, error) {
	if _, err := s.repo.Find(ctx, reg, name); err != nil {
		return nil, err
	}
	n, err := s.images.CountUsing(ctx, reg, name)
	if err != nil {
		return nil, err
	}
	res := &RemoveResult{Name: name, Images: n}
	if n > 0 {
		if err := s.repo.SetActive(ctx, reg, name, false); err != nil {
			return nil, err
		}
		res.Deactivated = true
		return res, nil
	}
	if err := s.repo.Delete(ctx, reg, name); err != nil {
		return nil, err
	}
	res.Deleted = true
	return res, nil
}
