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

// Package é um lote de imagens enviado junto. Vira uma pasta em dataset/.
type Package struct {
	Name      string    `bson:"name" json:"name"`
	CreatedBy string    `bson:"created_by" json:"created_by"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// PackageSummary é o pacote com o progresso de verificação.
type PackageSummary struct {
	Package    `bson:",inline"`
	Total      int64 `bson:"total" json:"total"`
	Verified   int64 `bson:"verified" json:"verified"`
	Pending    int64 `bson:"pending" json:"pending"`
	InProgress int64 `bson:"in_progress" json:"in_progress"` // pendentes que alguém está anotando agora
}

// CreatePackage cadastra um pacote (nome já sanitizado).
func (s *Store) CreatePackage(ctx context.Context, name, createdBy string) error {
	_, err := s.db.Collection("packages").InsertOne(ctx, Package{
		Name:      name,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
	})
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate // índice único em packages.name
	}
	if err != nil {
		return fmt.Errorf("criar pacote %s: %w", name, err)
	}
	return nil
}

// FindPackage busca um pacote pelo nome.
func (s *Store) FindPackage(ctx context.Context, name string) (*Package, error) {
	var p Package
	err := s.db.Collection("packages").FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar pacote %s: %w", name, err)
	}
	return &p, nil
}

// ListPackages devolve todos os pacotes (mais novos primeiro) com o progresso.
//
// O progresso vem de UMA agregação sobre images, agrupando por pacote e status:
//
//	{ _id: { package: "p1", status: "pending" },  n: 12 }
//	{ _id: { package: "p1", status: "verified" }, n: 30 }
//
// e depois juntamos com a lista de pacotes (pacote vazio aparece com zeros).
func (s *Store) ListPackages(ctx context.Context) ([]PackageSummary, error) {
	cur, err := s.db.Collection("packages").Find(ctx, bson.D{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("listar pacotes: %w", err)
	}
	var pkgs []Package
	if err := cur.All(ctx, &pkgs); err != nil {
		return nil, fmt.Errorf("ler pacotes: %w", err)
	}

	// claimed: conta 1 se claimed_until > agora (campo ausente nunca é > data).
	now := time.Now().UTC()
	pipeline := mongo.Pipeline{
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

	type counts struct{ verified, pending, inProgress int64 }
	byPkg := map[string]*counts{}
	for _, g := range groups {
		c := byPkg[g.ID.Package]
		if c == nil {
			c = &counts{}
			byPkg[g.ID.Package] = c
		}
		switch g.ID.Status {
		case StatusVerified:
			c.verified += g.N
		case StatusPending:
			c.pending += g.N
			c.inProgress += g.Claimed
		}
	}

	out := make([]PackageSummary, 0, len(pkgs))
	for _, p := range pkgs {
		sum := PackageSummary{Package: p}
		if c := byPkg[p.Name]; c != nil {
			sum.Verified, sum.Pending, sum.InProgress = c.verified, c.pending, c.inProgress
			sum.Total = c.verified + c.pending
		}
		out = append(out, sum)
	}
	return out, nil
}
