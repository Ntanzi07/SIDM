// Opções dos campos do perfil veiculos, para os FILTROS e as PASTAS do export
// (que ainda usam os campos fixos). A tela de anotação usa o perfil de
// anotação do pacote (GET /annotation-profiles/{nome}), não esta lista.
//
// Opções dos campos codificados: [valor da API, rótulo em português].
//
// ATENÇÃO: a API trabalha com os VALORES das tabelas ("rear", "night",
// "1920x1080"), não com as letras do nome do arquivo ("T", "N", "08"). A
// conversão para as letras é feita pelo backend, só na hora de montar o nome
// v2. Os valores aqui precisam ser os mesmos de
// backend/internal/filename/codes.go (o lado direito de cada tabela), que
// também aparecem no GET /codes. A resolução vem direto do GET /codes.
type Options = readonly (readonly [string, string])[]

export const annotationOptions = {
  direction: [
    ['front', 'Frontal'],
    ['rear', 'Traseira'],
  ],
  capture_angle: [
    ['lateral', 'Lateral à via'],
    ['overhead', 'Sobre a via'],
  ],
  day_time: [
    ['day', 'Diurno'],
    ['night', 'Noturno'],
  ],
  image_range: [
    ['narrow', 'Estreito (1 placa)'],
    ['wide', 'Amplo (várias placas)'],
  ],
  weather: [
    ['clear', 'Sem chuva'],
    ['rain', 'Chuva'],
    ['fog', 'Neblina'],
  ],
  condition: [
    ['no_damage', 'Sem avarias'],
    ['plate_shadow', 'Sombra na placa'],
    ['duplicate_plate', 'Placa duplicada (caminhões)'],
    ['distorted_plate', 'Placa distorcida'],
    ['glare', 'Ofuscamento'],
    ['low_resolution', 'Baixa resolução'],
    ['low_contrast', 'Baixo contraste'],
    ['distorted_image', 'Imagem distorcida'],
    ['multiple_plates', 'Diversas placas'],
  ],
  type: [
    ['car', 'Carro'],
    ['motorcycle', 'Moto'],
    ['truck', 'Caminhão'],
    ['bus', 'Ônibus'],
  ],
  plate_type: [
    ['br', 'Brasileira (antiga)'],
    ['mercosul', 'Mercosul'],
  ],
  plate_category: [
    ['rental', 'Aluguel'],
    ['learner', 'Aprendizagem'],
    ['collection', 'Coleção'],
    ['diplomatic', 'Diplomática'],
    ['official_special', 'Especial (oficiais)'],
    ['manufacturer_test', 'Experiência/fabricante'],
    ['government', 'Oficial (governo)'],
    ['private', 'Particular'],
  ],
  color: [
    ['yellow', 'Amarela'],
    ['blue', 'Azul'],
    ['beige', 'Bege'],
    ['white', 'Branca'],
    ['gray', 'Cinza'],
    ['golden', 'Dourada'],
    ['maroon', 'Grená'],
    ['orange', 'Laranja'],
    ['brown', 'Marrom'],
    ['silver', 'Prata'],
    ['black', 'Preta'],
    ['pink', 'Rosa'],
    ['purple', 'Roxa'],
    ['green', 'Verde'],
    ['red', 'Vermelha'],
    ['multicolor', 'Fantasia'],
  ],
} as const satisfies Record<string, Options>
