package annotation

import (
	"sort"

	"SIDM/internal/filename"
)

// DefaultProfile é o perfil de quem não escolhe outro.
const DefaultProfile = "veiculos"

// Os nomes em português das opções do perfil "veiculos". Os valores e os
// códigos vêm das tabelas do filename (as do nome v2), para nunca saírem de
// sincronia; aqui ficam só os nomes da tela.
var vehicleLabels = map[string]map[string]string{
	"direction":     {"front": "Dianteira", "rear": "Traseira"},
	"capture_angle": {"lateral": "Lateral", "overhead": "Sobre a via"},
	"day_time":      {"day": "Dia", "night": "Noite"},
	"image_range":   {"narrow": "Estreito (1 placa)", "wide": "Amplo (várias placas)"},
	"weather":       {"clear": "Limpo", "rain": "Chuva", "fog": "Neblina"},
	"conditions": {
		"no_damage": "Sem avarias", "plate_shadow": "Sombra na placa", "duplicate_plate": "Placa duplicada",
		"distorted_plate": "Placa distorcida", "glare": "Reflexo", "low_resolution": "Baixa resolução",
		"low_contrast": "Baixo contraste", "distorted_image": "Imagem distorcida", "multiple_plates": "Várias placas",
	},
	"plate_type": {"br": "BR (antiga)", "mercosul": "Mercosul"},
	"plate_category": {
		"rental": "Aluguel", "learner": "Aprendizagem", "collection": "Coleção", "diplomatic": "Diplomática",
		"official_special": "Especial (oficial)", "manufacturer_test": "Experiência (fabricante)",
		"government": "Oficial", "private": "Particular",
	},
	"vehicle_type": {"car": "Carro", "motorcycle": "Moto", "truck": "Caminhão", "bus": "Ônibus"},
	"vehicle_color": {
		"yellow": "Amarela", "blue": "Azul", "beige": "Bege", "white": "Branca", "gray": "Cinza",
		"golden": "Dourada", "maroon": "Grená", "orange": "Laranja", "brown": "Marrom", "silver": "Prata",
		"black": "Preta", "pink": "Rosa", "purple": "Roxa", "green": "Verde", "red": "Vermelha",
		"multicolor": "Fantasia",
	},
}

// codedOptions monta as opções de um campo a partir da tabela do nome v2
// (código -> valor), em ordem de código.
func codedOptions(table string) []Option {
	codes := filename.Codes()[table]
	out := make([]Option, 0, len(codes))
	for code, value := range codes {
		label := vehicleLabels[table][value]
		if label == "" {
			label = value // ex.: resolução, "1920x1080"
		}
		out = append(out, Option{Value: value, Label: label, Code: code})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Vehicles é o perfil "veiculos": os campos que o SIDM sempre teve, com as
// mesmas chaves que as imagens já usam no banco (para a migração ser só
// mudar de lugar) e os mesmos códigos do nome v2.
func Vehicles() Profile {
	choice := func(key, label, table string) Field {
		return Field{Key: key, Label: label, Type: TypeChoice, Options: codedOptions(table)}
	}
	return Profile{
		Name:        DefaultProfile,
		Description: "Veículos e placas (o padrão do SIDM, com o nome de arquivo v2)",
		Builtin:     true,
		Fields: []Field{
			{Key: "equipment", Label: "Equipamento", Type: TypeRef, Ref: RefEquipment},
			{Key: "camera", Label: "Câmera", Type: TypeRef, Ref: RefCamera},
			choice("direction", "Direção", "direction"),
			choice("resolution", "Resolução", "resolution"),
			choice("capture_angle", "Ângulo de captura", "capture_angle"),
			choice("day_time", "Dia/noite", "day_time"),
			choice("image_range", "Alcance da imagem", "image_range"),
			choice("weather", "Clima", "weather"),
			{Key: "conditions", Label: "Condições", Type: TypeMulti, Options: codedOptions("conditions")},
			{Key: "captured_at", Label: "Data e hora da captura", Type: TypeDateTime},
		},
		Objects: []ObjectType{{
			Key: "vehicle", Label: "Veículo",
			Fields: []Field{
				{Key: "plate", Label: "Placa", Type: TypeText, Pattern: "[A-Z0-9]{7}", Upper: true,
					Help: "ABC1234 (BR) ou ABC1D23 (Mercosul); vazio se ilegível"},
				choice("plate_type", "Tipo de placa", "plate_type"),
				choice("plate_category", "Categoria da placa", "plate_category"),
				choice("type", "Tipo de veículo", "vehicle_type"),
				choice("color", "Cor", "vehicle_color"),
			},
		}},
	}
}

// Builtins são os perfis do código (não ficam no banco).
func Builtins() []Profile { return []Profile{Vehicles()} }

// Builtin devolve o perfil padrão com esse nome, se houver.
func Builtin(name string) (Profile, bool) {
	for _, p := range Builtins() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}
