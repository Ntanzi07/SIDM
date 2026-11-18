package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/platform"
)

// MongoRepo guarda o catálogo nas coleções packages, equipments e cameras
// (os índices únicos de nome são criados pelo 01-init.js).
type MongoRepo struct {
	db *mongo.Database
}

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) packages() *mongo.Collection { return m.db.Collection("packages") }

func (m *MongoRepo) CreatePackage(ctx context.Context, p Package) error {
	_, err := m.packages().InsertOne(ctx, p)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate // índice único em packages.name
	}
	if err != nil {
		return fmt.Errorf("criar pacote %s: %w", p.Name, err)
	}
	return nil
}

func (m *MongoRepo) FindPackage(ctx context.Context, name string) (*Package, error) {
	var p Package
	err := m.packages().FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar pacote %s: %w", name, err)
	}
	return &p, nil
}

func (m *MongoRepo) ListPackages(ctx context.Context, q string, limit, skip int64) ([]Package, int64, error) {
	filter := bson.D{}
	if q != "" {
		filter = append(filter, platform.NameContains(q))
	}
	total, err := m.packages().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar pacotes: %w", err)
	}
	cur, err := m.packages().Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "name", Value: 1}}).
		SetLimit(limit).
		SetSkip(skip))
	if err != nil {
		return nil, 0, fmt.Errorf("listar pacotes: %w", err)
	}
	pkgs := []Package{}
	if err := cur.All(ctx, &pkgs); err != nil {
		return nil, 0, fmt.Errorf("ler pacotes: %w", err)
	}
	return pkgs, total, nil
}

func (m *MongoRepo) RenamePackage(ctx context.Context, oldName, newName string) error {
	res, err := m.packages().UpdateOne(ctx,
		bson.D{{Key: "name", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: newName}}}})
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("renomear pacote %s: %w", oldName, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) DeletePackage(ctx context.Context, name string) error {
	res, err := m.packages().DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar pacote %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// ---------- equipamentos e câmeras ----------

// Search: ativos cujo nome CONTÉM q (sem diferenciar maiúscula), em ordem
// alfabética, paginado. q vazio = todos. "mf47" encontra "8003-MF4739".
func (m *MongoRepo) Search(ctx context.Context, reg Registry, q string, limit, skip int64) ([]string, int64, error) {
	coll := m.db.Collection(string(reg))
	filter := bson.D{{Key: "active", Value: true}}
	if q != "" {
		filter = append(filter, platform.NameContains(q))
	}
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar %s: %w", reg, err)
	}
	cur, err := coll.Find(ctx, filter, options.Find().
		SetProjection(bson.D{{Key: "name", Value: 1}}). // só traz o campo name
		SetSort(bson.D{{Key: "name", Value: 1}}).
		SetLimit(limit).
		SetSkip(skip))
	if err != nil {
		return nil, 0, fmt.Errorf("listar %s: %w", reg, err)
	}
	var docs []struct {
		Name string `bson:"name"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("ler %s: %w", reg, err)
	}
	names := make([]string, 0, len(docs)) // [] e não null no JSON
	for _, d := range docs {
		names = append(names, d.Name)
	}
	return names, total, nil
}

func (m *MongoRepo) Create(ctx context.Context, reg Registry, name string) error {
	_, err := m.db.Collection(string(reg)).InsertOne(ctx, bson.D{
		{Key: "name", Value: name},
		{Key: "active", Value: true},
		{Key: "created_at", Value: time.Now().UTC()},
	})
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate // quem garante isso é o índice único do 01-init.js
	}
	if err != nil {
		return fmt.Errorf("inserir em %s: %w", reg, err)
	}
	return nil
}

func (m *MongoRepo) Find(ctx context.Context, reg Registry, name string) (*RegistryEntry, error) {
	var e RegistryEntry
	err := m.db.Collection(string(reg)).FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&e)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar %s %s: %w", reg, name, err)
	}
	return &e, nil
}

func (m *MongoRepo) ActiveExists(ctx context.Context, reg Registry, name string) (bool, error) {
	n, err := m.db.Collection(string(reg)).CountDocuments(ctx,
		bson.D{{Key: "name", Value: name}, {Key: "active", Value: true}},
		options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("consultar %s: %w", reg, err)
	}
	return n > 0, nil
}

// SetActive ativa ou desativa um item. Desativado, ele some do autocomplete
// e não pode ser usado em novas verificações, mas as imagens que já o usam
// continuam intactas.
func (m *MongoRepo) SetActive(ctx context.Context, reg Registry, name string, active bool) error {
	res, err := m.db.Collection(string(reg)).UpdateOne(ctx,
		bson.D{{Key: "name", Value: name}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "active", Value: active}}}})
	if err != nil {
		return fmt.Errorf("atualizar %s %s: %w", reg, name, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// Delete apaga o item de vez. Quem chama garante que ninguém usa.
func (m *MongoRepo) Delete(ctx context.Context, reg Registry, name string) error {
	res, err := m.db.Collection(string(reg)).DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar %s %s: %w", reg, name, err)
	}
	if res.DeletedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}
