Crie um frontend React + TypeScript + Vite para um sistema de anotação de imagens de placas veiculares.

O objetivo não é copiar exatamente o design visual, mas replicar a arquitetura, a organização das pastas, os componentes, os campos da anotação e o fluxo funcional.

## Stack

- React 18
- TypeScript
- Vite
- lucide-react para ícones
- papaparse para leitura de CSV
- Persistência usando localStorage
- Não usar backend neste primeiro momento

## Tipos principais

Crie os seguintes tipos:

type Code = string;

type Annotation = {
  direction: Code;
  capture_angle: Code;
  day_time: Code;
  vehicle_type: Code;
  image_range: Code;
  plate_type: Code;
  plate_category: Code;
  weather: Code;
  condition: Code;
  plate: string;
};

type ImageItem = {
  id: string;
  file: File;
  url: string;
  plate: string;
  vehicle: Code;
  resolution?: Code;
};

type CsvRow = {
  imagem?: string;
  image?: string;
  placa?: string;
  veiculo?: string;
};

type ImageFilter = "all" | "pending" | "done";

## Campos da anotação

O formulário deve conter estes campos:

1. Direção
   - F = Frontal
   - T = Traseiro

2. Ângulo de captura
   - L = Lateral sobre a via
   - S = Sobre a via

3. Período
   - D = Diurno
   - N = Noturno

4. Tipo de veículo
   - C = Car
   - M = Motorcycle
   - T = Truck
   - B = Bus

5. Alcance da imagem
   - E = Estreito
   - A = Amplo

6. Tipo da placa
   - B = Brasileira
   - M = Mercosul

7. Categoria da placa
   - 1 = Aluguel
   - 2 = Aprendizagem
   - 3 = Coleção
   - 4 = Diplomática
   - 5 = Especial (oficiais)
   - 6 = EXP Fabricante
   - 7 = Oficial (governo)
   - 8 = Particular

8. Clima
   - S = Sem avarias
   - C = Chuva
   - N = Neblina

9. Condição
   - 01 = Sem avarias
   - 02 = Sombra na placa
   - 03 = Placa duplicada (caminhões)
   - 04 = Placa distorcida
   - 05 = Ofuscamento
   - 06 = Baixa resolução
   - 07 = Baixo contraste
   - 08 = Imagem distorcida
   - 09 = Diversas placas

10. Placa
   - Campo de texto editável
   - Converter automaticamente para letras maiúsculas
   - Obrigatório para salvar

## Valores padrão

Use estes valores iniciais:

{
  direction: "T",
  capture_angle: "L",
  day_time: "D",
  vehicle_type: "C",
  image_range: "E",
  plate_type: "B",
  plate_category: "8",
  weather: "S",
  condition: "01",
  plate: ""
}

## Resoluções

Detecte a resolução real da imagem usando naturalWidth e naturalHeight.

Mapeie as resoluções:

- 640x480 = 01
- 752x480 = 02
- 800x600 = 03
- 1024x768 = 04
- 1280x720 = 05
- 1280x960 = 06
- 1712x876 = 07
- 1920x1080 = 08
- 2488x2048 = 09
- 4096x2160 = 10

## Nome gerado

Mostre uma prévia do nome final da imagem usando o formato:

40 zeros + direction + resolution + capture_angle + day_time +
vehicle_type + image_range + plate_type + plate_category +
weather + condition + plate

Exemplo:

0000000000000000000000000000000000000000T02LDBEB1S01ABC1234.jpg

Caso a resolução ainda não tenha sido detectada, use "??".

## Fluxo da aplicação

A tela principal deve conter:

### Cabeçalho

- Nome do sistema
- Status indicando que o estado foi salvo localmente
- Botões visuais para ajuda e atalhos

### Barra de importação

Três cartões:

1. Importar imagens
   - Aceitar múltiplos arquivos
   - Aceitar apenas arquivos de imagem
   - Mostrar quantidade carregada

2. Importar CSV
   - Opcional
   - Mostrar o nome do arquivo carregado

3. Destino
   - Informar que o progresso é salvo no navegador

### Fila de imagens

Criar uma lista lateral com:

- Miniatura da imagem
- Nome do arquivo
- Status:
  - Aguardando revisão
  - Anotação concluída
- Imagem selecionada destacada
- Campo de busca por nome
- Filtros:
  - Todas
  - Pendentes
  - Concluídas
- Botão para exportar o estado em JSON
- Botão para limpar a sessão
- Estado vazio com botão para selecionar arquivos

### Visualização

Criar uma área central com:

- Preview da imagem selecionada
- Suporte para drag and drop
- Nome do arquivo
- Tamanho e tipo do arquivo
- Indicador de posição, por exemplo: "2 de 10"
- Botões Anterior e Próxima
- Mensagens de status
- Detecção automática da resolução

### Formulário de anotação

Criar um painel lateral com:

- Select para cada campo codificado
- Campo de placa
- Nome final gerado
- Botão "Salvar anotação"
- Desabilitar o formulário quando nenhuma imagem estiver selecionada

## Importação do CSV

Aceite CSV com as colunas:

imagem,placa,veiculo

Exemplo:

imagem,placa,veiculo
imagem_01.jpg,ABC1234,C
imagem_02.jpg,DEF5678,M

Também aceite os nomes alternativos:

- image no lugar de imagem
- placa
- veiculo

Associe cada linha pelo nome exato do arquivo.

Ao importar:

- Preencher a placa da imagem
- Preencher o tipo de veículo
- Converter os valores para maiúsculo
- Informar a quantidade de registros importados
- Exibir erro caso o CSV não possa ser lido

## Regras de comportamento

- Não adicionar a mesma imagem duas vezes.
- Criar uma URL temporária com URL.createObjectURL.
- Liberar as URLs com URL.revokeObjectURL ao limpar a sessão.
- Salvar as anotações no localStorage.
- Usar a chave "splanotador.annotations".
- Ao abrir uma imagem já anotada, carregar sua anotação salva.
- Ao salvar, validar todos os campos e a placa.
- Remover espaços da placa e converter para maiúsculo.
- Após salvar, avançar automaticamente para a próxima imagem.
- O progresso deve mostrar:
  - Quantidade concluída
  - Quantidade total
  - Percentual concluído
- Permitir exportar todas as anotações como "anotacoes.json".
- Confirmar antes de limpar a sessão.
- Quando o nome do arquivo contiver um horário no formato HHMMSS, inferir:
  - 06:00 até 17:59 = Diurno
  - Outros horários = Noturno
- Se não for possível identificar o horário, usar Diurno.

## Organização do código

- Coloque os tipos em types/annotation.ts.
- Coloque opções, valores padrão, resoluções e constantes em data/annotationOptions.ts.
- Centralize o estado e as regras no hook hooks/useAnnotator.ts.
- Mantenha os componentes visuais pequenos e independentes.
- A página AnnotatorPage deve apenas conectar o hook aos componentes.
- Evite colocar regras de negócio diretamente no JSX.
- Use TypeScript estrito e evite any.
- Crie uma interface funcional, mas não é necessário reproduzir exatamente o design original.