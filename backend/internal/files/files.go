// Package files cuida das imagens no disco:
//
//	<root>/tmp/      uploads em andamento
//	<root>/staging/  <sha1>.<ext>             aguardando anotação (pending)
//	<root>/dataset/  <nome v2 completo>.<ext> anotadas (reviewed)
//
// tmp fica dentro de root de propósito: assim o os.Rename de tmp para
// staging é no mesmo disco, ou seja, instantâneo e atômico (nunca existe
// um arquivo "pela metade" em staging).
package files

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

type Storage struct {
	root string
}

// NewStorage cria as pastas necessárias (se ainda não existirem).
func NewStorage(root string) (*Storage, error) {
	for _, dir := range []string{"tmp", "staging", "dataset"} {
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

// StagingPath é o caminho RELATIVO (com "/", igual em Windows e Linux)
// que vai salvo no banco.
func StagingPath(sum, ext string) string {
	return path.Join("staging", sum+ext)
}

// DatasetPath é o caminho relativo de uma imagem anotada.
func DatasetPath(fileName string) string {
	return path.Join("dataset", fileName)
}

// Move move um arquivo entre dois caminhos relativos (ex: staging -> dataset).
func (st *Storage) Move(fromRel, toRel string) error {
	if err := os.Rename(st.Abs(fromRel), st.Abs(toRel)); err != nil {
		return fmt.Errorf("mover %s -> %s: %w", fromRel, toRel, err)
	}
	return nil
}

// Stage move o temporário para staging/<sha1>.<ext>.
func (st *Storage) Stage(tmpPath, sum, ext string) error {
	dst := st.Abs(StagingPath(sum, ext))
	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("mover para staging: %w", err)
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
