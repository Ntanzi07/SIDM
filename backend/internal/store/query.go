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

// ErrNotFound indica que o documento não existe.
var ErrNotFound = errors.New("não encontrado")

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

// ReplaceImage grava o documento inteiro por cima do atual.
func (s *Store) ReplaceImage(ctx context.Context, img *Image) error {
	res, err := s.db.Collection("images").ReplaceOne(ctx, bson.D{{Key: "_id", Value: img.SHA1}}, img)
	if err != nil {
		return fmt.Errorf("atualizar imagem %s: %w", img.SHA1, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ImageFilter: campos vazios são ignorados.
type ImageFilter struct {
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
// Ordem: mais antigas primeiro (a fila de anotação segue a ordem de chegada).
func (s *Store) ListImages(ctx context.Context, f ImageFilter) ([]Image, int64, error) {
	coll := s.db.Collection("images")
	filter := f.toBSON()

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar imagens: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "ingested_at", Value: 1}, {Key: "_id", Value: 1}}).
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

// EquipmentExists / CameraExists: o nome está cadastrado e ativo?
func (s *Store) EquipmentExists(ctx context.Context, name string) (bool, error) {
	return s.activeNameExists(ctx, "equipments", name)
}

func (s *Store) CameraExists(ctx context.Context, name string) (bool, error) {
	return s.activeNameExists(ctx, "cameras", name)
}

func (s *Store) activeNameExists(ctx context.Context, collection, name string) (bool, error) {
	n, err := s.db.Collection(collection).CountDocuments(ctx,
		bson.D{{Key: "name", Value: name}, {Key: "active", Value: true}},
		options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("consultar %s: %w", collection, err)
	}
	return n > 0, nil
}
