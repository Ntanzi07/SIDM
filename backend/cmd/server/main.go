package main

import (
	"log"
	"net/http"
	"os"
)

// Ponto de entrada do servidor SIDM.
// TODO: carregar config, conectar no Mongo (internal/store) e registrar as rotas (internal/api).
func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Printf("SIDM ouvindo em %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
