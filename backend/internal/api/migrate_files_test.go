package api

import (
	"context"
	"os"
	"testing"

	"SIDM/internal/images"
)

// Uma verificada de antes (arquivo com o nome v2 no disco) volta para
// <sha1>.jpg; rodar de novo não faz nada; arquivo sumido vira aviso.
func TestMigrateFiles(t *testing.T) {
	env := newTestEnv(t)
	sha := env.seedVerified(t, "verificada")
	pending := env.seedPending(t, "pendente")
	lost := env.seedVerified(t, "sumiu")

	// o estado de antes: o arquivo da verificada com o nome v2
	for _, id := range []string{sha, lost} {
		img := env.img.Images[id]
		old := "dataset/" + testPkg + "/" + img.FileName
		if err := os.Rename(env.pkgFile(id+".jpg"), env.pkgFile(img.FileName)); err != nil {
			t.Fatal(err)
		}
		env.img.Update(id, func(i *images.Image) { i.Path = old })
	}
	os.Remove(env.pkgFile(env.img.Images[lost].FileName)) // e um que sumiu do disco

	ctx := context.Background()
	moved, failed, err := env.imgSvc.MigrateFiles(ctx)
	if err != nil || moved != 1 || len(failed) != 1 {
		t.Fatalf("MigrateFiles = %d, %v, %v", moved, failed, err)
	}
	img := env.img.Images[sha]
	if img.Path != "dataset/"+testPkg+"/"+sha+".jpg" {
		t.Errorf("path = %q", img.Path)
	}
	if _, err := os.Stat(env.pkgFile(sha + ".jpg")); err != nil {
		t.Error("o arquivo não voltou para <sha1>.jpg")
	}
	if img.FileName == "" || img.PatternVersion != "v2" {
		t.Errorf("o nome v2 fica no banco: %q %q", img.FileName, img.PatternVersion)
	}
	if env.img.Images[pending].Path != "dataset/"+testPkg+"/"+pending+".jpg" {
		t.Error("a pendente não devia mudar")
	}

	// de novo: só o que falhou é tentado (e falha de novo)
	if moved, failed, _ := env.imgSvc.MigrateFiles(ctx); moved != 0 || len(failed) != 1 {
		t.Errorf("segunda vez = %d, %v", moved, failed)
	}
}
