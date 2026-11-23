package versions

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/platform"
)

// MongoRepo são as coleções datasets, versions e version_items.
type MongoRepo struct{ db *mongo.Database }

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) datasets() *mongo.Collection { return m.db.Collection("datasets") }
func (m *MongoRepo) versions() *mongo.Collection { return m.db.Collection("versions") }
func (m *MongoRepo) items() *mongo.Collection    { return m.db.Collection("version_items") }

// EnsureIndexes: nome do dataset único; número único por dataset (é o que
// segura duas versões criadas ao mesmo tempo); os itens por versão e sha1.
// Idempotente (roda a cada subida).
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := m.datasets().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("índice de datasets: %w", err)
	}
	if _, err := m.versions().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "dataset", Value: 1}, {Key: "number", Value: -1}}, Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("índice de versions: %w", err)
	}
	if _, err := m.items().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "version", Value: 1}, {Key: "sha1", Value: 1}}, Options: options.Index().SetUnique(true),
	}); err != nil {
		return fmt.Errorf("índice de version_items: %w", err)
	}
	return nil
}

func (m *MongoRepo) CreateDataset(ctx context.Context, d *Dataset) error {
	_, err := m.datasets().InsertOne(ctx, d)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar dataset %s: %w", d.Name, err)
	}
	return nil
}

func (m *MongoRepo) FindDataset(ctx context.Context, name string) (*Dataset, error) {
	var d Dataset
	err := m.datasets().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar dataset %s: %w", name, err)
	}
	return &d, nil
}

func (m *MongoRepo) ListDatasets(ctx context.Context) ([]Dataset, error) {
	cur, err := m.datasets().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar datasets: %w", err)
	}
	list := []Dataset{}
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("ler datasets: %w", err)
	}
	return list, nil
}

func (m *MongoRepo) ReplaceDataset(ctx context.Context, d *Dataset) error {
	res, err := m.datasets().ReplaceOne(ctx, bson.D{{Key: "name", Value: d.Name}}, d)
	if err != nil {
		return fmt.Errorf("salvar dataset %s: %w", d.Name, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) DeleteDataset(ctx context.Context, name string) error {
	res, err := m.datasets().DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar dataset %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) LastVersion(ctx context.Context, dataset string) (*Version, error) {
	var v Version
	err := m.versions().FindOne(ctx, bson.D{{Key: "dataset", Value: dataset}},
		options.FindOne().SetSort(bson.D{{Key: "number", Value: -1}})).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("última versão de %s: %w", dataset, err)
	}
	return &v, nil
}

func (m *MongoRepo) InsertVersion(ctx context.Context, v *Version) error {
	_, err := m.versions().InsertOne(ctx, v)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("gravar a versão %d de %s: %w", v.Number, v.Dataset, err)
	}
	return nil
}

func (m *MongoRepo) FindVersion(ctx context.Context, dataset string, number int) (*Version, error) {
	var v Version
	err := m.versions().FindOne(ctx, bson.D{{Key: "dataset", Value: dataset}, {Key: "number", Value: number}}).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar a versão %d de %s: %w", number, dataset, err)
	}
	return &v, nil
}

func (m *MongoRepo) ListVersions(ctx context.Context, dataset string) ([]Version, error) {
	cur, err := m.versions().Find(ctx, bson.D{{Key: "dataset", Value: dataset}},
		options.Find().SetSort(bson.D{{Key: "number", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("listar as versões de %s: %w", dataset, err)
	}
	list := []Version{}
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("ler as versões de %s: %w", dataset, err)
	}
	return list, nil
}

func (m *MongoRepo) InsertItems(ctx context.Context, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	docs := make([]any, len(items))
	for i := range items {
		docs[i] = items[i]
	}
	if _, err := m.items().InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("gravar as imagens da versão: %w", err)
	}
	return nil
}

func (m *MongoRepo) DeleteItems(ctx context.Context, version string) error {
	if _, err := m.items().DeleteMany(ctx, bson.D{{Key: "version", Value: version}}); err != nil {
		return fmt.Errorf("apagar as imagens da versão %s: %w", version, err)
	}
	return nil
}

func (m *MongoRepo) ListItems(ctx context.Context, version string, limit, skip int64) ([]Item, int64, error) {
	filter := bson.D{{Key: "version", Value: version}}
	total, err := m.items().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar as imagens da versão: %w", err)
	}
	cur, err := m.items().Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "sha1", Value: 1}}).SetLimit(limit).SetSkip(skip))
	if err != nil {
		return nil, 0, fmt.Errorf("listar as imagens da versão: %w", err)
	}
	list := []Item{}
	if err := cur.All(ctx, &list); err != nil {
		return nil, 0, fmt.Errorf("ler as imagens da versão: %w", err)
	}
	return list, total, nil
}
