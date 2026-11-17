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

// Registry é um dos cadastros simples: equipamentos ou câmeras.
// Os dois funcionam igual, só mudam a collection e o campo na imagem.
type Registry string

const (
	Equipments Registry = "equipments"
	Cameras    Registry = "cameras"
)

// ImageField é o campo da imagem que aponta para este cadastro.
func (r Registry) ImageField() string {
	if r == Cameras {
		return "camera"
	}
	return "equipment"
}

// RegistryEntry é um item de cadastro.
type RegistryEntry struct {
	Name   string `bson:"name" json:"name"`
	Active bool   `bson:"active" json:"active"`
}

// FindRegistry busca um item (ativo ou não).
func (s *Store) FindRegistry(ctx context.Context, reg Registry, name string) (*RegistryEntry, error) {
	var e RegistryEntry
	err := s.db.Collection(string(reg)).FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&e)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar %s %s: %w", reg, name, err)
	}
	return &e, nil
}

// SetRegistryActive ativa ou desativa um item. Desativado, ele some do
// autocomplete e não pode ser usado em novas verificações, mas as imagens
// que já o usam continuam intactas.
func (s *Store) SetRegistryActive(ctx context.Context, reg Registry, name string, active bool) error {
	res, err := s.db.Collection(string(reg)).UpdateOne(ctx,
		bson.D{{Key: "name", Value: name}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "active", Value: active}}}})
	if err != nil {
		return fmt.Errorf("atualizar %s %s: %w", reg, name, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRegistry apaga o item de vez. Quem chama garante que ninguém usa.
func (s *Store) DeleteRegistry(ctx context.Context, reg Registry, name string) error {
	res, err := s.db.Collection(string(reg)).DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar %s %s: %w", reg, name, err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// CountImagesUsing conta as imagens (de qualquer status) que usam o item.
func (s *Store) CountImagesUsing(ctx context.Context, reg Registry, name string) (int64, error) {
	n, err := s.db.Collection("images").CountDocuments(ctx, bson.D{{Key: reg.ImageField(), Value: name}})
	if err != nil {
		return 0, fmt.Errorf("contar imagens com %s %s: %w", reg.ImageField(), name, err)
	}
	return n, nil
}

// RenameOnPending troca o nome nas imagens PENDENTES, de uma vez só.
// Nas pendentes o nome não está no arquivo (que ainda se chama <sha1>.jpg),
// então basta o banco. O updated_at muda junto: quem estiver com a imagem
// aberta recebe 409 ao salvar, e recarrega com o nome novo.
func (s *Store) RenameOnPending(ctx context.Context, reg Registry, oldName, newName string, now time.Time) (int64, error) {
	field := reg.ImageField()
	res, err := s.db.Collection("images").UpdateMany(ctx,
		bson.D{{Key: field, Value: oldName}, {Key: "status", Value: StatusPending}},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: newName}, {Key: "updated_at", Value: now}}}})
	if err != nil {
		return 0, fmt.Errorf("renomear %s nas pendentes: %w", field, err)
	}
	return res.ModifiedCount, nil
}

// ---------- pacotes ----------

// RenamePackage troca o nome do pacote no cadastro e em todas as imagens
// dele (o campo package e o começo do path: dataset/<antigo>/ vira
// dataset/<novo>/). Quem chama renomeia a pasta no disco.
func (s *Store) RenamePackage(ctx context.Context, oldName, newName string) (int64, error) {
	res, err := s.db.Collection("packages").UpdateOne(ctx,
		bson.D{{Key: "name", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: newName}}}})
	if mongo.IsDuplicateKeyError(err) {
		return 0, ErrDuplicate
	}
	if err != nil {
		return 0, fmt.Errorf("renomear pacote %s: %w", oldName, err)
	}
	if res.MatchedCount == 0 {
		return 0, ErrNotFound
	}

	// Update com pipeline: monta o path novo a partir do antigo, documento a
	// documento, numa operação só (sem trazer as imagens para a aplicação).
	oldPrefix := "dataset/" + oldName + "/"
	pipeline := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: "package", Value: newName},
		{Key: "path", Value: bson.D{{Key: "$concat", Value: bson.A{
			"dataset/" + newName + "/",
			bson.D{{Key: "$substrCP", Value: bson.A{"$path", len([]rune(oldPrefix)), 1 << 20}}},
		}}}},
	}}}}
	imgs, err := s.db.Collection("images").UpdateMany(ctx, bson.D{{Key: "package", Value: oldName}}, pipeline)
	if err != nil {
		return 0, fmt.Errorf("renomear o pacote nas imagens: %w", err)
	}
	if _, err := s.boxes().UpdateMany(ctx, bson.D{{Key: "package", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "package", Value: newName}}}}); err != nil {
		return imgs.ModifiedCount, fmt.Errorf("renomear o pacote nas caixas: %w", err)
	}
	return imgs.ModifiedCount, nil
}

// DeletePackage apaga o pacote, todas as imagens dele e o histórico delas.
// Os arquivos no disco ficam por conta de quem chama.
// Devolve quantas imagens foram apagadas.
func (s *Store) DeletePackage(ctx context.Context, name string) (int64, error) {
	images := s.db.Collection("images")
	filter := bson.D{{Key: "package", Value: name}}

	// 1. Os sha1 das imagens, para apagar o histórico delas (em lotes).
	cur, err := images.Find(ctx, filter, options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return 0, fmt.Errorf("listar imagens do pacote %s: %w", name, err)
	}
	var docs []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return 0, fmt.Errorf("ler imagens do pacote %s: %w", name, err)
	}
	const batch = 1000
	for start := 0; start < len(docs); start += batch {
		end := min(start+batch, len(docs))
		ids := make([]string, 0, end-start)
		for _, d := range docs[start:end] {
			ids = append(ids, d.ID)
		}
		if _, err := s.db.Collection("image_history").DeleteMany(ctx,
			bson.D{{Key: "image_id", Value: bson.D{{Key: "$in", Value: ids}}}}); err != nil {
			return 0, fmt.Errorf("apagar histórico do pacote %s: %w", name, err)
		}
	}

	// 2. As caixas (CVAT) e as imagens.
	if _, err := s.boxes().DeleteMany(ctx, filter); err != nil {
		return 0, fmt.Errorf("apagar caixas do pacote %s: %w", name, err)
	}
	res, err := images.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("apagar imagens do pacote %s: %w", name, err)
	}

	// 3. O pacote por último: se algo falhar antes, ele continua existindo
	// e a exclusão pode ser repetida.
	pkg, err := s.db.Collection("packages").DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return res.DeletedCount, fmt.Errorf("apagar pacote %s: %w", name, err)
	}
	if pkg.DeletedCount == 0 && res.DeletedCount == 0 {
		return 0, ErrNotFound
	}
	return res.DeletedCount, nil
}
