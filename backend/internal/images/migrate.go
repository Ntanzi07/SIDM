package images

import (
	"SIDM/internal/files"
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Os campos de antes dos perfis de anotação, que ficavam soltos no
// documento da imagem. Agora são os campos do perfil veiculos, em fields; os
// veículos são os objetos "vehicle", em objects.
var (
	legacyImageFields   = []string{"equipment", "camera", "direction", "resolution", "capture_angle", "day_time", "image_range", "weather", "conditions", "captured_at"}
	legacyVehicleFields = []string{"plate", "plate_type", "plate_category", "type", "color"}
)

// nonEmptyPairs monta, para o $arrayToObject, os pares [nome, valor] dos
// campos que existem e não estão vazios (nem "", nem [], nem null).
func nonEmptyPairs(prefix string, keys []string) bson.D {
	pairs := bson.A{}
	for _, k := range keys {
		pairs = append(pairs, bson.A{k, prefix + k})
	}
	return bson.D{{Key: "$arrayToObject", Value: bson.D{{Key: "$filter", Value: bson.D{
		{Key: "input", Value: pairs},
		{Key: "as", Value: "p"},
		{Key: "cond", Value: bson.D{{Key: "$not", Value: bson.A{
			bson.D{{Key: "$in", Value: bson.A{
				bson.D{{Key: "$ifNull", Value: bson.A{bson.D{{Key: "$arrayElemAt", Value: bson.A{"$$p", 1}}}, nil}}},
				bson.A{nil, "", bson.A{}},
			}}},
		}}}},
	}}}}}
}

// Migrate leva as imagens de antes dos perfis para fields/objects, numa
// operação só no banco (update com pipeline; nada passa pela aplicação).
// Idempotente: só pega quem ainda não tem fields. Devolve quantas migrou.
func (m *MongoRepo) Migrate(ctx context.Context) (int64, error) {
	unset := append([]string{"vehicles"}, legacyImageFields...)
	pipeline := mongo.Pipeline{
		{{Key: "$set", Value: bson.D{
			{Key: "fields", Value: nonEmptyPairs("$", legacyImageFields)},
			{Key: "objects", Value: bson.D{{Key: "$map", Value: bson.D{
				{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$vehicles", bson.A{}}}}},
				{Key: "as", Value: "v"},
				{Key: "in", Value: bson.D{
					{Key: "type", Value: "vehicle"},
					{Key: "fields", Value: nonEmptyPairs("$$v.", legacyVehicleFields)},
				}},
			}}}},
		}}},
		{{Key: "$unset", Value: unset}},
	}
	res, err := m.db.Collection("images").UpdateMany(ctx,
		bson.D{{Key: "fields", Value: bson.D{{Key: "$exists", Value: false}}}}, pipeline)
	if err != nil {
		return 0, fmt.Errorf("migrar as imagens para os campos do perfil: %w", err)
	}
	return res.ModifiedCount, nil
}

// EnsureIndexes: os índices de busca nos campos novos (os de antes, nos
// campos soltos, saem: ninguém mais procura por eles). Idempotente.
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	coll := m.db.Collection("images")
	for _, old := range []string{"equipment_1_captured_at_-1", "camera_1_captured_at_-1", "day_time_1_captured_at_-1", "weather_1_captured_at_-1"} {
		_ = coll.Indexes().DropOne(ctx, old) // já apagado (ou nunca criado): tudo bem
	}
	_, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "fields.equipment", Value: 1}}},
		{Keys: bson.D{{Key: "fields.camera", Value: 1}}},
		{Keys: bson.D{{Key: "fields.day_time", Value: 1}}},
		{Keys: bson.D{{Key: "fields.weather", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("índices de images: %w", err)
	}
	return nil
}

// MigrateFiles leva para <sha1>.ext os arquivos das verificadas que ainda
// têm o nome v2 no disco (de antes de o nome ficar só no banco). Uma a uma:
// move o arquivo e grava o caminho novo; se o banco falhar, o arquivo volta.
// Idempotente: quem já está no lugar não casa com a busca. Devolve quantas
// moveu e as que falharam (essas ficam como estão e são tentadas na próxima
// subida).
func (s *Service) MigrateFiles(ctx context.Context) (moved int, failed []string, err error) {
	const pageSize = 200
	for skip := int64(0); ; skip += pageSize {
		page, _, err := s.repo.ListImages(ctx, ImageFilter{Status: StatusVerified, Limit: pageSize, Skip: skip})
		if err != nil {
			return moved, failed, err
		}
		for i := range page {
			img := &page[i]
			want := files.PendingPath(img.Package, img.SHA1, img.Ext)
			if img.Path == want {
				continue
			}
			before := *img
			if err := s.files.Move(img.Path, want); err != nil {
				failed = append(failed, fmt.Sprintf("%s (%v)", img.SHA1, err))
				continue
			}
			img.Path, img.UpdatedAt = want, s.now()
			if err := s.repo.ReplaceImage(ctx, img, before.UpdatedAt); err != nil {
				_ = s.files.Move(want, before.Path) // banco e disco nunca divergem
				failed = append(failed, fmt.Sprintf("%s (%v)", img.SHA1, err))
				continue
			}
			moved++
		}
		if len(page) < pageSize {
			return moved, failed, nil
		}
	}
}
