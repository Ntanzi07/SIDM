package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// FindImage busca uma imagem pelo sha1.
func (s *Store) FindImage(ctx context.Context, sha1 string) (*Image, error) {
	var img Image
	err := s.db.Collection("images").FindOne(ctx, bson.D{{Key: "_id", Value: sha1}}).Decode(&img)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar imagem %s: %w", sha1, err)
	}
	return &img, nil
}

// ErrConflict indica que o documento mudou desde que foi lido.
var ErrConflict = errors.New("alterado por outra pessoa")

// ReplaceImage grava o documento inteiro por cima do atual, MAS só se ele
// ainda estiver como foi lido (updated_at igual a prevUpdatedAt).
//
// Isso é "optimistic locking": em vez de travar a imagem enquanto alguém
// edita, deixamos todo mundo ler, e na hora de gravar conferimos se ninguém
// gravou no meio. Se gravou, devolvemos ErrConflict e o front recarrega.
func (s *Store) ReplaceImage(ctx context.Context, img *Image, prevUpdatedAt time.Time) error {
	filter := bson.D{{Key: "_id", Value: img.SHA1}, {Key: "updated_at", Value: prevUpdatedAt}}
	res, err := s.db.Collection("images").ReplaceOne(ctx, filter, img)
	if err != nil {
		return fmt.Errorf("atualizar imagem %s: %w", img.SHA1, err)
	}
	if res.MatchedCount == 0 {
		// Não achou com aquele updated_at: ou a imagem sumiu, ou mudou.
		if _, err := s.FindImage(ctx, img.SHA1); errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

// ImageFilter: campos vazios são ignorados.
type ImageFilter struct {
	Package      string
	Status       string
	Equipment    string
	Camera       string
	DayTime      string
	Weather      string
	Direction    string
	VehicleType  string
	VehicleColor string
	Plate        string
	Limit        int64
	Skip         int64
}

func (f ImageFilter) toBSON() bson.D {
	filter := bson.D{}
	add := func(field, value string) {
		if value != "" {
			filter = append(filter, bson.E{Key: field, Value: value})
		}
	}
	add("package", f.Package)
	add("status", f.Status)
	add("equipment", f.Equipment)
	add("camera", f.Camera)
	add("day_time", f.DayTime)
	add("weather", f.Weather)
	add("direction", f.Direction)

	// Filtros de veículo usam $elemMatch: todas as condições precisam valer
	// para o MESMO veículo. Sem isso, "caminhão branco" traria uma imagem
	// com um caminhão preto e um carro branco.
	vehicle := bson.D{}
	if f.VehicleType != "" {
		vehicle = append(vehicle, bson.E{Key: "type", Value: f.VehicleType})
	}
	if f.VehicleColor != "" {
		vehicle = append(vehicle, bson.E{Key: "color", Value: f.VehicleColor})
	}
	if f.Plate != "" {
		vehicle = append(vehicle, bson.E{Key: "plate", Value: f.Plate})
	}
	if len(vehicle) > 0 {
		filter = append(filter, bson.E{Key: "vehicles", Value: bson.D{{Key: "$elemMatch", Value: vehicle}}})
	}
	return filter
}

// ListImages devolve uma página de resultados e o total que bate com o filtro.
// Ordem: a mesma da fila de trabalho (queued_at), mais antigas primeiro.
func (s *Store) ListImages(ctx context.Context, f ImageFilter) ([]Image, int64, error) {
	coll := s.db.Collection("images")
	filter := f.toBSON()

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar imagens: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "queued_at", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(f.Limit).
		SetSkip(f.Skip)

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("listar imagens: %w", err)
	}
	items := []Image{}
	if err := cur.All(ctx, &items); err != nil {
		return nil, 0, fmt.Errorf("ler imagens: %w", err)
	}
	return items, total, nil
}

// HistoryEntry é um documento da collection image_history.
type HistoryEntry struct {
	ImageID   string    `bson:"image_id" json:"image_id"`
	Field     string    `bson:"field" json:"field"`
	Old       any       `bson:"old" json:"old"`
	New       any       `bson:"new" json:"new"`
	ChangedBy string    `bson:"changed_by" json:"changed_by"`
	ChangedAt time.Time `bson:"changed_at" json:"changed_at"`
}

// InsertHistory grava várias entradas de uma vez (nada a fazer se vazio).
func (s *Store) InsertHistory(ctx context.Context, entries []HistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if _, err := s.db.Collection("image_history").InsertMany(ctx, entries); err != nil {
		return fmt.Errorf("gravar histórico: %w", err)
	}
	return nil
}

// ExistingImage diz onde uma imagem já cadastrada está e em que estado.
type ExistingImage struct {
	Package string
	Status  string
}

// ExistingImages recebe uma lista de sha1 e devolve quais já existem,
// com pacote e status de cada um. Uma consulta só ($in), não uma por hash.
func (s *Store) ExistingImages(ctx context.Context, ids []string) (map[string]ExistingImage, error) {
	filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}
	opts := options.Find().SetProjection(bson.D{{Key: "status", Value: 1}, {Key: "package", Value: 1}}) // _id vem sempre

	cur, err := s.db.Collection("images").Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("consultar hashes: %w", err)
	}
	var docs []struct {
		ID      string `bson:"_id"`
		Package string `bson:"package"`
		Status  string `bson:"status"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("ler hashes: %w", err)
	}

	found := make(map[string]ExistingImage, len(docs))
	for _, d := range docs {
		found[d.ID] = ExistingImage{Package: d.Package, Status: d.Status}
	}
	return found, nil
}
