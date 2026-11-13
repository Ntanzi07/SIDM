// Package crypt abre as imagens criptografadas pelos equipamentos.
//
// Dois formatos existem nos equipamentos (ver o pacote crypto do met):
//
//   - RC4 legado: o arquivo inteiro passa por RC4 com uma chave FIXA, o
//     SHA-256 de um único byte zero. Não precisa de segredo para abrir.
//   - PGP: a imagem é cifrada com uma chave simétrica (S2), e a S2 vai no
//     final do arquivo, trancada para as chaves públicas P1 e P2. Precisa
//     da chave PRIVADA P1 ou P2. Ainda não suportado aqui.
//
// O resultado da descriptografia é o arquivo ORIGINAL do equipamento
// (JPEG + INI assinado), byte a byte.
package crypt

import (
	"bytes"
	"crypto/rc4"
	"crypto/sha256"
	"errors"
)

const (
	FormatPlain     = ""           // já era um arquivo aberto
	FormatRC4Legacy = "rc4-legacy" // RC4 com a chave fixa do equipamento
)

var (
	// ErrPGPUnsupported: arquivo no formato PGP (precisa da chave privada).
	ErrPGPUnsupported = errors.New("arquivo criptografado no formato PGP: ainda não suportado (precisa da chave privada P1/P2)")
	// ErrNotEncrypted: não é JPEG aberto nem um formato criptografado conhecido.
	ErrNotEncrypted = errors.New("não é um formato criptografado conhecido")
)

// A "chave" do formato legado: SHA-256 de um único byte zero, igual ao
// pacote crypto do equipamento (password = [1]byte{0}).
var legacyKey = sha256.Sum256([]byte{0})

var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pgpMarker = []byte("|-----BEGIN PGP MESSAGE-----")
)

// IsJPEG diz se os dados já são um JPEG aberto.
func IsJPEG(data []byte) bool {
	return bytes.HasPrefix(data, jpegMagic)
}

// Open descobre o formato e devolve o arquivo aberto.
//
//	JPEG aberto        -> (data, FormatPlain, nil)
//	RC4 legado         -> (original, FormatRC4Legacy, nil)
//	PGP                -> (nil, "", ErrPGPUnsupported)
//	qualquer outra     -> (nil, "", ErrNotEncrypted)
//
// Os dados de entrada não são alterados.
func Open(data []byte) ([]byte, string, error) {
	if IsJPEG(data) {
		return data, FormatPlain, nil
	}
	if bytes.Contains(data, pgpMarker) {
		return nil, "", ErrPGPUnsupported
	}
	if plain, ok := tryRC4Legacy(data); ok {
		return plain, FormatRC4Legacy, nil
	}
	return nil, "", ErrNotEncrypted
}

// tryRC4Legacy aplica o RC4 legado e confere se virou JPEG.
// A conferência é o que torna seguro "tentar": num arquivo que não é RC4,
// a chance de o resultado começar com FF D8 FF por acaso é de 1 em 16 milhões.
func tryRC4Legacy(data []byte) ([]byte, bool) {
	c, err := rc4.NewCipher(legacyKey[:])
	if err != nil {
		return nil, false
	}
	out := make([]byte, len(data)) // cópia: nunca alteramos a entrada
	c.XORKeyStream(out, data)
	if !IsJPEG(out) {
		return nil, false
	}
	return out, true
}

// EncryptRC4Legacy faz o caminho inverso (RC4 é simétrico). Só existe para
// os testes montarem um arquivo igual ao do equipamento.
func EncryptRC4Legacy(plain []byte) []byte {
	c, _ := rc4.NewCipher(legacyKey[:])
	out := make([]byte, len(plain))
	c.XORKeyStream(out, plain)
	return out
}
