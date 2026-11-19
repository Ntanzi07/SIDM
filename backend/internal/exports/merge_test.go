package exports

import (
	"testing"

	"SIDM/internal/boxes"
)

func TestMergeSets(t *testing.T) {
	byTask := []map[string]boxes.Set{
		{"a": {Width: 100, Height: 50, Boxes: []boxes.Box{{Label: "parabrisa", X: 10, Y: 10, W: 20, H: 10}}}},
		{}, // "a" não está nesta tarefa
		// a mesma imagem no CVAT com a metade do tamanho: a caixa dobra
		{"a": {Width: 50, Height: 25, Boxes: []boxes.Box{{Label: "placa", X: 5, Y: 5, W: 4, H: 2}}}},
	}
	set, found := mergeSets("a", byTask)
	if found != 2 || set.Width != 100 || len(set.Boxes) != 2 {
		t.Fatalf("found=%d set=%+v", found, set)
	}
	if b := set.Boxes[1]; b.X != 10 || b.Y != 10 || b.W != 8 || b.H != 4 {
		t.Errorf("caixa da outra tarefa fora de escala: %+v", b)
	}
	if _, found := mergeSets("z", byTask); found != 0 {
		t.Error("z não está em nenhuma tarefa")
	}
	// o Set da primeira tarefa não pode mudar (o append é numa cópia)
	if len(byTask[0]["a"].Boxes) != 1 {
		t.Error("mergeSets mexeu no Set original")
	}
}
