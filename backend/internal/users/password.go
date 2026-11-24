package users

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// A senha vira "pbkdf2-sha256$<iterações>$<sal>$<hash>" (base64). As
// iterações ficam no texto: dá para aumentar depois sem invalidar as senhas
// antigas.
const (
	hashPrefix = "pbkdf2-sha256"
	saltBytes  = 16
	keyBytes   = 32
)

// HashIterations: 600 mil, a recomendação da OWASP para PBKDF2-SHA256. Só
// os testes baixam (senão cada cadastro de teste leva meio segundo).
var HashIterations = 600_000

// MinPassword: o tamanho mínimo da senha.
const MinPassword = 8

func hashPassword(password string) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, HashIterations, keyBytes)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", hashPrefix, HashIterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// checkPassword compara em tempo constante. Um hash em formato
// desconhecido não confere.
func checkPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash: comparado quando a conta não existe, para o login demorar o
// mesmo (o tempo não pode dizer quais nomes existem).
var (
	dummyOnce sync.Once
	dummy     string
)

func dummyHash() string {
	dummyOnce.Do(func() { dummy, _ = hashPassword("sidm-nao-existe") })
	return dummy
}

// newToken: 32 bytes aleatórios (o cookie) e o sha256 deles (o _id no banco).
func newToken() (token, id string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, tokenID(token), nil
}

func tokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
