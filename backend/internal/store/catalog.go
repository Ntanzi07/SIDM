package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/catalog"
)

// O lado das IMAGENS e das CAIXAS das operações do catálogo (pacotes,
// equipamentos e câmeras): o catálogo cuida das coleções dele e chama estas
// funções (pelas interfaces catalog.Images e catalog.PackageListener).

// PackageProgress conta as imagens de cada pacote por status, numa
// agregação só, sobre os pacotes pedidos:
//
//	{ _id: { package: "p1", status: "pending" },  n: 12, claimed: 2 }
//	{ _id: { package: "p1", status: "verified" }, n: 30, claimed: 0 }
//
// Pacote sem imagens não aparece no mapa (quem chama usa zeros).
func (s *Store) PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error) {
	// claimed: conta 1 se claimed_until > agora (campo ausente nunca é > data).
	now := time.Now().UTC()
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "package", Value: bson.D{{Key: "$in", Value: names}}}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "package", Value: "$package"}, {Key: "status", Value: "$status"}}},
			{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "claimed", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$gt", Value: bson.A{"$claimed_until", now}}}, 1, 0,
			}}}}}},
		}}},
	}
	agg, err := s.db.Collection("images").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("progresso dos pacotes: %w", err)
	}
	var groups []struct {
		ID struct {
			Package string `bson:"package"`
			Status  string `bson:"status"`
		} `bson:"_id"`
		N       int64 `bson:"n"`
		Claimed int64 `bson:"claimed"`
	}
	if err := agg.All(ctx, &groups); err != nil {
		return nil, fmt.Errorf("ler progresso: %w", err)
	}
	out := map[string]catalog.Progress{}
	for _, g := range groups {
		p := out[g.ID.Package]
		switch g.ID.Status {
		case StatusVerified:
			p.Verified += g.N
		case StatusPending:
			p.Pending += g.N
			p.InProgress += g.Claimed
		}
		p.Total = p.Verified + p.Pending
		out[g.ID.Package] = p
	}
	return out, nil
}

// RenamePackageImages troca o pacote em todas as imagens dele (o campo
// package e o começo do path: dataset/<antigo>/ vira dataset/<novo>/).
func (s *Store) RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error) {
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
	res, err := s.db.Collection("images").UpdateMany(ctx, bson.D{{Key: "package", Value: oldName}}, pipeline)
	if err != nil {
		return 0, fmt.Errorf("renomear o pacote nas imagens: %w", err)
	}
	return res.ModifiedCount, nil
}

// DeletePackageImages apaga todas as imagens do pacote e o histórico delas.
// Os arquivos no disco ficam por conta de quem chama. Devolve quantas
// imagens foram apagadas.
func (s *Store) DeletePackageImages(ctx context.Context, name string) (int64, error) {
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

	// 2. As imagens (o histórico antes: se falhar aqui, repetir termina).
	res, err := images.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("apagar imagens do pacote %s: %w", name, err)
	}
	return res.DeletedCount, nil
}

// CountUsing conta as imagens (de qualquer status) que usam o item.
func (s *Store) CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error) {
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
func (s *Store) RenameOnPending(ctx context.Context, reg catalog.Registry, oldName, newName string, now time.Time) (int64, error) {
	field := reg.ImageField()
	res, err := s.db.Collection("images").UpdateMany(ctx,
		bson.D{{Key: field, Value: oldName}, {Key: "status", Value: StatusPending}},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: newName}, {Key: "updated_at", Value: now}}}})
	if err != nil {
		return 0, fmt.Errorf("renomear %s nas pendentes: %w", field, err)
	}
	return res.ModifiedCount, nil
}

// PackageRenamed: as caixas guardam o pacote de cada imagem.
func (s *Store) PackageRenamed(ctx context.Context, oldName, newName string) error {
	if _, err := s.boxes().UpdateMany(ctx, bson.D{{Key: "package", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "package", Value: newName}}}}); err != nil {
		return fmt.Errorf("renomear o pacote nas caixas: %w", err)
	}
	return nil
}

// PackageDeleted apaga as caixas das imagens do pacote.
func (s *Store) PackageDeleted(ctx context.Context, name string) error {
	if _, err := s.boxes().DeleteMany(ctx, bson.D{{Key: "package", Value: name}}); err != nil {
		return fmt.Errorf("apagar caixas do pacote %s: %w", name, err)
	}
	return nil
}
