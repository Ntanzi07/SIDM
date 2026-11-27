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
	"direction":     {"front": "Frontal", "rear": "Traseira"},
	"capture_angle": {"lateral": "Lateral à via", "overhead": "Sobre a via"},
	"day_time":      {"day": "Diurno", "night": "Noturno"},
	"image_range":   {"narrow": "Estreito (1 placa)", "wide": "Amplo (várias placas)"},
	"weather":       {"clear": "Sem chuva", "rain": "Chuva", "fog": "Neblina"},
	"conditions": {
		"no_damage": "Sem avarias", "plate_shadow": "Sombra na placa", "duplicate_plate": "Placa duplicada (caminhões)",
		"distorted_plate": "Placa distorcida", "glare": "Ofuscamento", "low_resolution": "Baixa resolução",
		"low_contrast": "Baixo contraste", "distorted_image": "Imagem distorcida", "multiple_plates": "Diversas placas",
	},
	"plate_type": {"br": "Brasileira (antiga)", "mercosul": "Mercosul"},
	"plate_category": {
		"rental": "Aluguel", "learner": "Aprendizagem", "collection": "Coleção", "diplomatic": "Diplomática",
		"official_special": "Especial (oficiais)", "manufacturer_test": "Experiência/fabricante",
		"government": "Oficial (governo)", "private": "Particular",
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
	choice := func(key, label, table string, def ...string) Field {
		f := Field{Key: key, Label: label, Type: TypeChoice, Options: codedOptions(table)}
		if len(def) > 0 {
			f.Default = def[0]
		}
		return f
	}
	repeat := func(f Field) Field {
		f.Repeat = true
		return f
	}
	// a resolução é a do arquivo: quem anota não escolhe
	fromImage := func(f Field) Field {
		f.FromImage = FromImageSize
		return f
	}
	return Profile{
		Name:        DefaultProfile,
		Description: "Veículos e placas (o padrão do SIDM, com o nome de arquivo v2)",
		Builtin:     true,
		Fields: []Field{
			// Repeat: equipamento, câmera, direção, ângulo e clima quase não
			// mudam de uma foto para a outra do mesmo pacote.
			{Key: "equipment", Label: "Equipamento", Type: TypeRef, Ref: RefEquipment, Repeat: true},
			{Key: "camera", Label: "Câmera", Type: TypeRef, Ref: RefCamera, Repeat: true},
			repeat(choice("direction", "Direção", "direction", "rear")),
			fromImage(choice("resolution", "Resolução", "resolution")), // quem dita é o arquivo
			repeat(choice("capture_angle", "Ângulo de captura", "capture_angle", "lateral")),
			choice("day_time", "Período", "day_time", "day"),
			choice("image_range", "Abrangência", "image_range", "narrow"),
			repeat(choice("weather", "Clima", "weather", "clear")),
			{Key: "conditions", Label: "Condições", Type: TypeMulti, Options: codedOptions("conditions"), Default: []string{"no_damage"}},
			{Key: "captured_at", Label: "Data e hora da captura", Type: TypeDateTime},
		},
		Objects: []ObjectType{{
			Key: "vehicle", Label: "Veículo",
			Fields: []Field{
				{Key: "plate", Label: "Placa", Type: TypeText, Pattern: "[A-Z0-9]{7}", MaxLen: 7, Upper: true,
					Help: "ABC1234 (BR) ou ABC1D23 (Mercosul); vazio se ilegível"},
				choice("plate_type", "Tipo de placa", "plate_type", "mercosul"),
				choice("plate_category", "Categoria da placa", "plate_category", "private"),
				choice("type", "Tipo de veículo", "vehicle_type", "car"),
				// sem valor inicial: é o que mais varia, e um padrão errado
				// passaria despercebido na revisão
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
