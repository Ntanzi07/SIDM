package importer

// Perfis de import que vêm com o SIDM, como os perfis padrão de exportação:
// ficam no código (não no banco), aparecem na lista para todo mundo e não
// podem ser editados nem apagados. Para mudar um, copie com outro nome.

// SpliceINI lê o INI que os equipamentos da Splice (MFR1) gravam depois do
// JPEG. A data e a hora são as do bloco [Metrologico] (a parte assinada);
// o resto vem do [NMetrologico].
func SpliceINI() Profile {
	return Profile{
		Name:              "splice-ini",
		Description:       "INI embutido nas imagens dos equipamentos Splice (MFR1)",
		AnnotationProfile: "veiculos",
		Builtin:           true,
		Reader:            ReaderConfig{Type: ReaderJPEGINI},
		Fields: map[string]FieldRule{
			"equipment": {Column: "NMetrologico.NSerie"},
			"captured_at": {
				Column:     "Metrologico.Data",
				TimeColumn: "Metrologico.Hora",
				MsColumn:   "Metrologico.Milisegundo",
				Timezone:   "America/Sao_Paulo",
			},
		},
		Objects: map[string]*ObjectRules{
			"vehicle": {
				Mode: ObjectsSingle,
				Fields: map[string]FieldRule{
					"plate": {Column: "NMetrologico.Placa"},
					// Classificação do Denatran -> tipo do perfil veiculos. Valor
					// que não estiver aqui fica vazio e aparece no relatório.
					"type": {Column: "NMetrologico.TipoVeiculo", Map: map[string]string{
						"AUTOMOVEL": "car", "CAMINHONETE": "car", "CAMIONETA": "car", "UTILITARIO": "car",
						"MOTOCICLETA": "motorcycle", "MOTONETA": "motorcycle", "CICLOMOTOR": "motorcycle",
						"TRICICLO": "motorcycle", "QUADRICICLO": "motorcycle",
						"CAMINHAO": "truck", "CAMINHAO TRATOR": "truck", "REBOQUE": "truck", "SEMI-REBOQUE": "truck",
						"ONIBUS": "bus", "MICROONIBUS": "bus", "MICRO-ONIBUS": "bus",
					}},
				},
			},
		},
	}
}

// Builtins são os perfis de import padrão.
func Builtins() []Profile { return []Profile{SpliceINI()} }

// Builtin devolve o perfil padrão com esse nome, se houver.
func Builtin(name string) (Profile, bool) {
	for _, p := range Builtins() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}
