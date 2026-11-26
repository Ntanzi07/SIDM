package images

// O espaço em disco do envio de zip. Sem conferir, um zip maior que o livre
// chegava "inteiro" e não abria: no Docker Desktop (Windows), a pasta montada
// passa pelo WSL e, com o disco cheio, a escrita pode se perder sem erro; o
// Truncate de cada parte deixava o arquivo do tamanho certo, com zeros no
// lugar do que faltou ("zip: not a valid zip file").

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// zipSpare: o que fica livre além do envio (o sistema e o banco precisam).
const zipSpare = 1 << 30

// NoSpaceError: não há disco para o envio (vira 507).
type NoSpaceError struct{ Msg string }

func (e *NoSpaceError) Error() string { return e.Msg }

// SetDiskFree troca a medição do livre no disco (só os testes usam).
func (s *Service) SetDiskFree(free func(path string) (uint64, error)) { s.diskFree = free }

func (s *Service) freeBytes() (uint64, bool) {
	measure := s.diskFree
	if measure == nil {
		measure = func(p string) (uint64, error) { _, free, err := diskSpace(p); return free, err }
	}
	free, err := measure(s.files.Abs("."))
	if err != nil {
		// sem conseguir medir, não bloqueia (o envio segue como antes)
		log.Printf("envio de zip: não deu para medir o disco: %v", err)
		return 0, false
	}
	return free, true
}

// needSpace falha se o disco não tem need bytes (mais a folga). what diz para
// que é ("o resto do zip e as imagens dele").
func (s *Service) needSpace(need int64, what string) error {
	free, ok := s.freeBytes()
	if !ok || need <= 0 || uint64(need)+zipSpare <= free {
		return nil
	}
	return &NoSpaceError{fmt.Sprintf(
		"falta espaço em disco: %s precisa de %s e o servidor tem %s livres (deixando 1 GB de folga). "+
			"Libere espaço (Configurar → Armazenamento) e escolha o mesmo zip de novo para continuar de onde parou.",
		what, humanBytes(uint64(need)), humanBytes(free))}
}

// zipRoom: o que um envio ainda vai ocupar: o resto do zip e, ao gravar, as
// imagens tiradas dele (JPEG quase não comprime: ~ o tamanho do zip).
func zipRoom(z *ZipUpload) int64 { return (z.Size - z.Received) + z.Size }

// "26,5 GB"
func humanBytes(n uint64) string {
	units := []string{"bytes", "KB", "MB", "GB", "TB"}
	v, i := float64(n), 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d bytes", n)
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", v, units[i]), ".", ",", 1)
}

// zeroTail: o fim do arquivo chegou zerado (onde fica o índice do zip)? É o
// sinal de escrita perdida com o disco cheio.
func zeroTail(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return false
	}
	n := min(st.Size(), 64<<10)
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, st.Size()-n); err != nil && err != io.EOF {
		return false
	}
	return bytes.Count(buf, []byte{0}) == len(buf)
}
