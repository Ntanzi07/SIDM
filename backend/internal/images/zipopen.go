package images

// Abrir o zip do envio, aceitando o zip64 com o "total de discos" 0.
//
// Zip acima de 4 GB (ou com mais de 65.535 itens) usa o zip64, achado por um
// "localizador" de 20 bytes logo antes do fim. Alguns programas de compactar
// gravam o total de discos dele como 0 (o certo é 1). O archive/zip do Go
// exige 1 e, sem isso, ignora o zip64 inteiro: um zip bom de 26 GB dava "zip:
// not a valid zip file" (o Python e o 7-Zip abrem). Aqui, quando o Go recusa
// e o localizador está assim, o zip é lido com o campo valendo 1, só na
// leitura: o arquivo no disco não muda.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const (
	eocdLen        = 22 // o fim do diretório central (sem o comentário)
	zip64LocLen    = 20 // o localizador do zip64
	maxZipComment  = 1<<16 - 1
	zip64LocDisks  = 16 // onde fica o total de discos no localizador
	eocdSignature  = "PK\x05\x06"
	zip64LocSignat = "PK\x06\x07"
)

// openZip abre o zip do disco. Feche o io.Closer quando terminar.
func openZip(name string) (*zip.Reader, io.Closer, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	r, err := zip.NewReader(f, st.Size())
	if errors.Is(err, zip.ErrFormat) {
		if loc, ok := zeroDiskLocator(f, st.Size()); ok {
			r, err = zip.NewReader(fixDisks{f, loc + zip64LocDisks}, st.Size())
		}
	}
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return r, f, nil
}

// zeroDiskLocator acha o localizador do zip64 com o total de discos 0 e
// devolve onde ele começa.
func zeroDiskLocator(r io.ReaderAt, size int64) (int64, bool) {
	n := min(size, eocdLen+maxZipComment+zip64LocLen)
	tail := make([]byte, n)
	if _, err := r.ReadAt(tail, size-n); err != nil && err != io.EOF {
		return 0, false
	}
	// o fim do diretório é a última assinatura dele (pode ter comentário depois)
	eocd := bytes.LastIndex(tail, []byte(eocdSignature))
	if eocd < zip64LocLen {
		return 0, false
	}
	loc := tail[eocd-zip64LocLen : eocd]
	if string(loc[:4]) != zip64LocSignat || binary.LittleEndian.Uint32(loc[zip64LocDisks:]) != 0 {
		return 0, false
	}
	return size - n + int64(eocd-zip64LocLen), true
}

// fixDisks lê o arquivo como está, mas com o total de discos (4 bytes em at)
// valendo 1.
type fixDisks struct {
	r  io.ReaderAt
	at int64
}

func (d fixDisks) ReadAt(p []byte, off int64) (int, error) {
	n, err := d.r.ReadAt(p, off)
	one := [4]byte{1, 0, 0, 0}
	for i, b := range one {
		if j := d.at + int64(i) - off; j >= 0 && j < int64(n) {
			p[j] = b
		}
	}
	return n, err
}
