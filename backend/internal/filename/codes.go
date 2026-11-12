package filename

import "maps"

// --- Atributos da imagem ---

var directionCodes = map[string]string{
	"F": "front",
	"T": "rear",
}

var resolutionCodes = map[string]string{
	"01": "640x480",
	"02": "752x480",
	"03": "800x600",
	"04": "1024x768",
	"05": "1280x720",
	"06": "1280x960",
	"07": "1712x876",
	"08": "1920x1080",
	"09": "2488x2048",
	"10": "4096x2160",
}

var captureAngleCodes = map[string]string{
	"L": "lateral",  // Lateral - sobre a via
	"S": "overhead", // Sobre a via
}

var dayTimeCodes = map[string]string{
	"D": "day",
	"N": "night",
}

var imageRangeCodes = map[string]string{
	"E": "narrow", // Estreito (1 placa/img)
	"A": "wide",   // Amplo (n placas/img)
}

// TODO: confirmar o significado de "S" (na tabela está "Sem avarias").
var weatherCodes = map[string]string{
	"S": "clear",
	"C": "rain",
	"N": "fog",
}

var conditionCodes = map[string]string{
	"01": "no_damage",
	"02": "plate_shadow",
	"03": "duplicate_plate", // caminhões
	"04": "distorted_plate",
	"05": "glare",
	"06": "low_resolution",
	"07": "low_contrast",
	"08": "distorted_image",
	"09": "multiple_plates",
}

const (
	maxEquipmentLen = 16
	maxCameraLen    = 32
	maxPackageLen   = 48 // vira nome de pasta: não entra no nome do arquivo
)

// --- Atributos do veículo ---

var plateTypeCodes = map[string]string{
	"B": "br",
	"M": "mercosul",
}

var plateCategoryCodes = map[string]string{
	"1": "rental",            // Aluguel
	"2": "learner",           // Aprendizagem
	"3": "collection",        // Coleção
	"4": "diplomatic",        // Diplomáticos
	"5": "official_special",  // Especial (oficiais)
	"6": "manufacturer_test", // EXP Fabricante
	"7": "government",        // Oficial (org. governo)
	"8": "private",           // Particular
}

var vehicleTypeCodes = map[string]string{
	"C": "car",
	"M": "motorcycle",
	"T": "truck",
	"B": "bus",
}

// Cores alinhadas com a lista do Renavam.
var colorCodes = map[string]string{
	"YLW": "yellow",     // Amarela
	"BLU": "blue",       // Azul
	"BGE": "beige",      // Bege
	"WHT": "white",      // Branca
	"GRY": "gray",       // Cinza
	"GLD": "golden",     // Dourada
	"MRN": "maroon",     // Grená
	"ORG": "orange",     // Laranja
	"BRN": "brown",      // Marrom
	"SLV": "silver",     // Prata
	"BLK": "black",      // Preta
	"PNK": "pink",       // Rosa
	"PRP": "purple",     // Roxa
	"GRN": "green",      // Verde
	"RED": "red",        // Vermelha
	"MLT": "multicolor", // Fantasia
}

// TODO (seu): o Format() vai precisar do caminho inverso ("front" -> "F").
// Escreva uma função que recebe um map e devolve o map invertido,
// em vez de duplicar cada tabela na mão.

// Codes retorna uma CÓPIA de todas as tabelas do padrão, indexadas pelo
// nome do campo (o mesmo usado no Mongo). Cópia porque map em Go é
// referência: quem alterar o retorno não pode estragar o parser.
func Codes() map[string]map[string]string {
	tables := map[string]map[string]string{
		"direction":      directionCodes,
		"resolution":     resolutionCodes,
		"capture_angle":  captureAngleCodes,
		"day_time":       dayTimeCodes,
		"image_range":    imageRangeCodes,
		"weather":        weatherCodes,
		"conditions":     conditionCodes,
		"plate_type":     plateTypeCodes,
		"plate_category": plateCategoryCodes,
		"vehicle_type":   vehicleTypeCodes,
		"vehicle_color":  colorCodes,
	}
	out := make(map[string]map[string]string, len(tables))
	for field, table := range tables {
		out[field] = maps.Clone(table)
	}
	return out
}
