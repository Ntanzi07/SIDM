// Package files cuida das imagens no disco:
//
//	<root>/tmp/                         uploads em andamento
//	<root>/dataset/<pacote>/<sha1>.jpg  pendente (ainda não verificada)
//	<root>/dataset/<pacote>/<nome v2>   verificada
//
// Cada imagem pertence a UM pacote, então a pasta do pacote mostra
// exatamente o que ele contém.
//
// tmp fica dentro de root de propósito: assim o os.Rename de tmp para a
// pasta do pacote é no mesmo disco, ou seja, instantâneo e atômico (nunca
// existe um arquivo "pela metade" no dataset).
package files

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Storage struct {
	root string
}

// NewStorage cria as pastas base (se ainda não existirem).
func NewStorage(root string) (*Storage, error) {
	for _, dir := range []string{"tmp", "dataset"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, fmt.Errorf("criar pasta %s: %w", dir, err)
		}
	}
	return &Storage{root: root}, nil
}

// Receive grava o conteúdo num arquivo temporário e calcula o SHA-1 AO
// MESMO TEMPO (io.MultiWriter): o arquivo é lido uma vez só, sem carregar
// tudo na memória.
func (st *Storage) Receive(r io.Reader) (tmpPath, sum string, size int64, err error) {
	f, err := os.CreateTemp(filepath.Join(st.root, "tmp"), "upload-*")
	if err != nil {
		return "", "", 0, fmt.Errorf("criar temporário: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()

	h := sha1.New()
	size, err = io.Copy(io.MultiWriter(f, h), r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("gravar upload: %w", err)
	}
	return f.Name(), hex.EncodeToString(h.Sum(nil)), size, nil
}

// PackageDir é a pasta RELATIVA de um pacote.
func PackageDir(pkg string) string {
	return path.Join("dataset", pkg)
}

// PendingPath é onde fica uma imagem ainda não verificada (nome = só o hash).
// Caminhos relativos usam "/" em qualquer sistema: é assim que vão para o banco.
func PendingPath(pkg, sum, ext string) string {
	return path.Join(PackageDir(pkg), sum+ext)
}

// VerifiedPath é onde fica uma imagem verificada (nome v2 completo).
func VerifiedPath(pkg, fileName string) string {
	return path.Join(PackageDir(pkg), fileName)
}

// EnsurePackageDir cria a pasta do pacote, se ainda não existir.
func (st *Storage) EnsurePackageDir(pkg string) error {
	if err := os.MkdirAll(st.Abs(PackageDir(pkg)), 0o755); err != nil {
		return fmt.Errorf("criar pasta do pacote %s: %w", pkg, err)
	}
	return nil
}

// RenamePackageDir renomeia a pasta do pacote (uma operação só, no mesmo disco).
// Se a pasta antiga não existir (pacote nunca recebeu arquivo), cria a nova.
func (st *Storage) RenamePackageDir(oldName, newName string) error {
	from, to := st.Abs(PackageDir(oldName)), st.Abs(PackageDir(newName))
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("a pasta %s já existe", PackageDir(newName))
	}
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return st.EnsurePackageDir(newName)
	}
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("renomear pasta %s -> %s: %w", oldName, newName, err)
	}
	return nil
}

// RemovePackageDir apaga a pasta do pacote com tudo dentro. IRREVERSÍVEL.
// Por segurança, só aceita um nome de pacote simples: nada de "..", "/" ou
// vazio, que fariam o RemoveAll sair de dentro de dataset/.
func (st *Storage) RemovePackageDir(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("nome de pacote inválido para apagar: %q", name)
	}
	if err := os.RemoveAll(st.Abs(PackageDir(name))); err != nil {
		return fmt.Errorf("apagar pasta do pacote %s: %w", name, err)
	}
	return nil
}

// Store move o temporário para o caminho relativo final (criando a pasta).
func (st *Storage) Store(tmpPath, rel string) error {
	dst := st.Abs(rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("criar pasta de %s: %w", rel, err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("guardar %s: %w", rel, err)
	}
	return nil
}

// Move move um arquivo entre dois caminhos relativos (ex: renomear para v2).
func (st *Storage) Move(fromRel, toRel string) error {
	if err := os.Rename(st.Abs(fromRel), st.Abs(toRel)); err != nil {
		return fmt.Errorf("mover %s -> %s: %w", fromRel, toRel, err)
	}
	return nil
}

// Discard apaga um temporário (upload duplicado ou com erro).
func (st *Storage) Discard(tmpPath string) {
	os.Remove(tmpPath)
}

// Abs converte o caminho relativo do banco no caminho real do disco.
func (st *Storage) Abs(rel string) string {
	return filepath.Join(st.root, filepath.FromSlash(rel))
}
