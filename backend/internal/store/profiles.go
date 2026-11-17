package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/importer"
)

// storedProfile é como o perfil fica no Mongo: o JSON inteiro num campo texto.
//
// Por que não salvar campo a campo? As chaves do "map" vêm do CSV ("Limpo",
// "S.A.", "$alguma coisa"...), e o Mongo tem restrições com chaves que têm
// "." ou começam com "$". Guardando o texto, qualquer valor funciona, e o
// que volta é exatamente o que foi escrito.
type storedProfile struct {
	Name      string    `bson:"name"`
	Spec      string    `bson:"spec"`
	CreatedBy string    `bson:"created_by,omitempty"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func toStored(p *importer.Profile) (storedProfile, error) {
	spec, err := json.Marshal(p)
	if err != nil {
		return storedProfile{}, fmt.Errorf("serializar perfil %s: %w", p.Name, err)
	}
	return storedProfile{Name: p.Name, Spec: string(spec), CreatedBy: p.CreatedBy, UpdatedAt: p.UpdatedAt}, nil
}

func (sp storedProfile) profile() (*importer.Profile, error) {
	var p importer.Profile
	if err := json.Unmarshal([]byte(sp.Spec), &p); err != nil {
		return nil, fmt.Errorf("perfil %s corrompido no banco: %w", sp.Name, err)
	}
	p.Name, p.CreatedBy, p.UpdatedAt = sp.Name, sp.CreatedBy, sp.UpdatedAt
	return &p, nil
}

// CreateProfile cadastra um perfil de import (já validado).
func (s *Store) CreateProfile(ctx context.Context, p *importer.Profile) error {
	doc, err := toStored(p)
	if err != nil {
		return err
	}
	_, err = s.db.Collection("import_profiles").InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate // índice único em import_profiles.name
	}
	if err != nil {
		return fmt.Errorf("criar perfil %s: %w", p.Name, err)
	}
	return nil
}

// FindProfile busca um perfil pelo nome.
func (s *Store) FindProfile(ctx context.Context, name string) (*importer.Profile, error) {
	var doc storedProfile
	err := s.db.Collection("import_profiles").FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar perfil %s: %w", name, err)
	}
	return doc.profile()
}

// ReplaceProfile grava a nova versão de um perfil existente.
// O created_by original é mantido.
func (s *Store) ReplaceProfile(ctx context.Context, p *importer.Profile) error {
	doc, err := toStored(p)
	if err != nil {
		return err
	}
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "spec", Value: doc.Spec},
		{Key: "updated_at", Value: doc.UpdatedAt},
	}}}
	res, err := s.db.Collection("import_profiles").UpdateOne(ctx, bson.D{{Key: "name", Value: p.Name}}, update)
	if err != nil {
		return fmt.Errorf("atualizar perfil %s: %w", p.Name, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameProfile troca o nome de um perfil. Nenhuma imagem guarda o nome do
// perfil (ele só é usado na hora do upload/import), então não há cascata.
func (s *Store) RenameProfile(ctx context.Context, oldName, newName string, now time.Time) error {
	res, err := s.db.Collection("import_profiles").UpdateOne(ctx,
		bson.D{{Key: "name", Value: oldName}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: newName}, {Key: "updated_at", Value: now}}}})
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("renomear perfil %s: %w", oldName, err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProfile apaga um perfil.
func (s *Store) DeleteProfile(ctx context.Context, name string) error {
	res, err := s.db.Collection("import_profiles").DeleteOne(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return fmt.Errorf("apagar perfil %s: %w", name, err)
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ListProfiles lista perfis em ordem alfabética, com busca por nome.
func (s *Store) ListProfiles(ctx context.Context, q string, limit, skip int64) ([]importer.Profile, int64, error) {
	coll := s.db.Collection("import_profiles")
	filter := bson.D{}
	if q != "" {
		filter = append(filter, nameContains(q))
	}

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("contar perfis: %w", err)
	}
	cur, err := coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}}).
		SetLimit(limit).
		SetSkip(skip))
	if err != nil {
		return nil, 0, fmt.Errorf("listar perfis: %w", err)
	}
	var docs []storedProfile
	if err := cur.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("ler perfis: %w", err)
	}

	out := make([]importer.Profile, 0, len(docs))
	for _, d := range docs {
		p, err := d.profile()
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	return out, total, nil
}

// FindImagesByKeys busca as imagens de um pacote (pkg "" = de todos) pela
// chave do import:
// by = "original_name" (nome do arquivo antes do upload) ou "sha1".
// Pode voltar mais de uma imagem por nome original (duas pastas diferentes
// com um "img001.jpg" cada): quem chama decide o que fazer.
// Consulta em lotes de 1000 chaves, para um CSV de 50 mil linhas não virar
// uma consulta gigante.
func (s *Store) FindImagesByKeys(ctx context.Context, pkg, by string, keys []string) ([]Image, error) {
	field := "original_name"
	if by == importer.MatchBySHA1 {
		field = "_id"
	}

	const batch = 1000
	out := []Image{}
	for start := 0; start < len(keys); start += batch {
		end := start + batch
		if end > len(keys) {
			end = len(keys)
		}
		filter := bson.D{{Key: field, Value: bson.D{{Key: "$in", Value: keys[start:end]}}}}
		if pkg != "" { // "" = em todos os pacotes (import de caixas numa tarefa)
			filter = append(filter, bson.E{Key: "package", Value: pkg})
		}
		cur, err := s.db.Collection("images").Find(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("buscar imagens do import: %w", err)
		}
		var imgs []Image
		if err := cur.All(ctx, &imgs); err != nil {
			return nil, fmt.Errorf("ler imagens do import: %w", err)
		}
		out = append(out, imgs...)
	}
	return out, nil
}
