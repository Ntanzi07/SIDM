package imports

// O import com andamento: com 70 mil linhas, o ensaio e (mais ainda) o
// aplicar levam minutos, e a tela ficava em "Ensaiando..." sem dizer nada.
// Quando ela pede (Accept: application/x-ndjson), a resposta vem aos poucos,
// uma linha JSON por vez:
//
//	{"progress": {"phase": "conferindo", "done": 12000, "total": 73174}}
//	...
//	{"report": {...}}            ou            {"error": "..."}
//
// Os erros de antes de começar (pacote, perfil, arquivo) continuam vindo
// como antes (400/404); um erro no meio vem na última linha. Sem o cabeçalho,
// a rota responde como sempre (o relatório inteiro no fim).

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

const (
	ndjson      = "application/x-ndjson"
	progressGap = 200 * time.Millisecond // o menor intervalo entre dois avisos da mesma fase
	errInternal = "erro interno"
)

type streamLine struct {
	Progress *Step   `json:"progress,omitempty"`
	Report   *Report `json:"report,omitempty"`
	Error    string  `json:"error,omitempty"`
}

func (s *Service) streamImport(w http.ResponseWriter, r *http.Request, run Run, table *importer.Table) {
	w.Header().Set("Content-Type", ndjson)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // o nginx repassa cada linha na hora
	w.WriteHeader(http.StatusOK)
	flusher := http.NewResponseController(w)
	enc := json.NewEncoder(w)
	send := func(line streamLine) {
		if err := enc.Encode(line); err == nil {
			flusher.Flush()
		}
	}

	var last Step
	var lastAt time.Time
	run.Progress = func(st Step) {
		// troca de fase e o fim de uma fase sempre vão; o resto, no máximo a cada progressGap
		if st.Phase == last.Phase && st.Done != st.Total && time.Since(lastAt) < progressGap {
			return
		}
		last, lastAt = st, time.Now()
		send(streamLine{Progress: &st})
	}

	rep, err := s.Import(r.Context(), run, table)
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		send(streamLine{Error: invalid.Msg})
	case err != nil:
		platform.LogError(err) // o detalhe fica no log; o cliente recebe "erro interno"
		send(streamLine{Error: errInternal})
	default:
		send(streamLine{Report: rep})
	}
}
