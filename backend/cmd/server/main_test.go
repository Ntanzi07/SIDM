package main

import (
	"net/url"
	"testing"
)

func TestGetenv(t *testing.T) {
	t.Setenv("SIDM_TESTE", "valor")
	if got := getenv("SIDM_TESTE", "padrão"); got != "valor" {
		t.Errorf("getenv = %q", got)
	}

	t.Setenv("SIDM_TESTE", "")
	if got := getenv("SIDM_TESTE", "padrão"); got != "padrão" {
		t.Errorf("variável vazia deveria usar o padrão, veio %q", got)
	}
}

func TestMongoURIFromEnv(t *testing.T) {
	// t.Setenv restaura o valor original no fim de cada teste.
	clear := func(t *testing.T) {
		for _, k := range []string{"MONGO_URI", "MONGO_APP_USER", "MONGO_APP_PASSWORD", "MONGO_PORT"} {
			t.Setenv(k, "")
		}
	}

	t.Run("MONGO_URI tem prioridade", func(t *testing.T) {
		clear(t)
		t.Setenv("MONGO_URI", "mongodb://docker:27017")
		t.Setenv("MONGO_APP_USER", "ignorado")
		if got := mongoURIFromEnv("sidm"); got != "mongodb://docker:27017" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("sem usuário: localhost sem auth", func(t *testing.T) {
		clear(t)
		if got := mongoURIFromEnv("sidm"); got != "mongodb://localhost:27017" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("com usuário e porta customizada", func(t *testing.T) {
		clear(t)
		t.Setenv("MONGO_APP_USER", "sidm_app")
		t.Setenv("MONGO_APP_PASSWORD", "segredo")
		t.Setenv("MONGO_PORT", "27018")
		want := "mongodb://sidm_app:segredo@localhost:27018/sidm?authSource=sidm"
		if got := mongoURIFromEnv("sidm"); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("senha com caracteres especiais é escapada", func(t *testing.T) {
		clear(t)
		t.Setenv("MONGO_APP_USER", "sidm_app")
		t.Setenv("MONGO_APP_PASSWORD", "p@ss:w/rd")
		got := mongoURIFromEnv("sidm")

		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("URI inválida %q: %v", got, err)
		}
		if pw, _ := u.User.Password(); pw != "p@ss:w/rd" {
			t.Errorf("senha não sobreviveu ao escape: %q (uri %q)", pw, got)
		}
		if u.Host != "localhost:27017" {
			t.Errorf("host corrompido pela senha: %q", u.Host)
		}
	})
}
