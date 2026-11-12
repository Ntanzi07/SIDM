package filename

import (
	"fmt"
	"strings"
)

// SanitizeEquipment converte o nome digitado no cadastro ("8003 MF4739")
// para a forma usada no nome do arquivo ("8003-MF4739").
func SanitizeEquipment(raw string) (string, error) {
	return sanitize(raw, maxEquipmentLen, "equipamento")
}

// SanitizeCamera faz o mesmo para câmeras ("Hikvision 233" -> "Hikvision-233").
func SanitizeCamera(raw string) (string, error) {
	return sanitize(raw, maxCameraLen, "câmera")
}

// sanitize: espaços (um ou vários) viram um único "-"; depois valida.
// Não "conserta" outros caracteres de propósito: melhor recusar "8003/MF"
// do que adivinhar o que a pessoa quis dizer.
func sanitize(raw string, max int, what string) (string, error) {
	s := strings.Join(strings.Fields(raw), "-")
	if s == "" {
		return "", fmt.Errorf("nome de %s vazio", what)
	}
	return validateName(s, max, what)
}
