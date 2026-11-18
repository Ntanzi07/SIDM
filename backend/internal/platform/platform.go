// Package platform é o que todos os módulos usam e não é de nenhum assunto:
// os erros comuns, as respostas HTTP (JSON, erro, paginação), quem é o
// usuário, a conexão com o Mongo e a busca por nome. Não depende de nenhum
// módulo.
package platform

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Erros comuns, para os módulos e os handlers conversarem sem conhecer o
// banco: errors.Is(err, platform.ErrNotFound) vira 404, ErrDuplicate vira 409.
var (
	ErrNotFound  = errors.New("não encontrado")
	ErrDuplicate = errors.New("já cadastrado")
)

// Route é uma rota: o módulo declara as suas, e o servidor registra todas
// (e o teste confere que cada uma está no openapi.yaml).
type Route struct {
	Method, Path string
	Handler      http.HandlerFunc
}

// Module é um pedaço do sistema que tem rotas HTTP.
type Module interface {
	Routes() []Route
}

// Page é o formato de toda listagem: {"total": N, "items": [...]}.
type Page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
}

// WriteJSON responde com JSON.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("erro ao escrever JSON: %v", err)
	}
}

// WriteError responde {"error": msg}.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ServerError loga o erro real e devolve uma mensagem genérica: detalhes
// internos (como erro do Mongo) não devem vazar para o cliente.
func ServerError(w http.ResponseWriter, err error) {
	log.Printf("erro interno: %v", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}

// LogError registra um erro que não interrompe a resposta.
func LogError(err error) {
	log.Printf("erro: %v", err)
}

// UserFrom diz quem está fazendo a ação.
// TODO: quando tiver login, o usuário vem da sessão, não de um header.
func UserFrom(r *http.Request) string {
	if u := r.Header.Get("X-User"); u != "" {
		return u
	}
	return "anonymous"
}

// IntParam lê um inteiro da query (?limit=20), com padrão e limites.
func IntParam(q url.Values, key string, def, minV, maxV int64) int64 {
	v, err := strconv.ParseInt(q.Get(key), 10, 64)
	if err != nil {
		return def
	}
	return min(max(v, minV), maxV)
}

// NameContains monta o filtro "name contém q, ignorando maiúscula".
//
// regexp.QuoteMeta é OBRIGATÓRIO: o texto vem do usuário, e sem escapar,
// alguém digitando ".*" ou "(a+)+$" estaria mandando uma regex para o
// Mongo (resultado errado, ou uma regex lenta de propósito travando o banco).
func NameContains(q string) bson.E {
	return bson.E{Key: "name", Value: bson.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}}
}
