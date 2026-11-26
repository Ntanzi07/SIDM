package images

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/catalog"
	"SIDM/internal/importer"
	"SIDM/internal/platform"
)

// MongoRepo guarda as imagens (images) e o histórico (image_history).
type MongoRepo struct {
	db *mongo.Database
}

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

// InsertImage grava uma imagem nova. Retorna ErrDuplicate se o sha1 já existe:
// como _id = sha1, o próprio Mongo impede a mesma imagem duas vezes.
func (m *MongoRepo) InsertImage(ctx context.Context, img *Image) error {
	_, err := m.db.Collection("images").InsertOne(ctx, img)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("inserir imagem: %w", err)
	}
	return nil
}

// DeleteImageHistory apaga o histórico de uma imagem (quando ela é excluída).
func (m *MongoRepo) DeleteImageHistory(ctx context.Context, sha1 string) error {
	_, err := m.db.Collection("image_history").DeleteMany(ctx, bson.D{{Key: "image_id", Value: sha1}})
	if err != nil {
		return fmt.Errorf("apagar histórico da imagem %s: %w", sha1, err)
	}
	return nil
}

// DeleteImage remove o documento. Usado ao excluir uma imagem e para desfazer
// um upload que falhou no disco.
func (m *MongoRepo) DeleteImage(ctx context.Context, sha1 string) error {
	_, err := m.db.Collection("images").DeleteOne(ctx, bson.D{{Key: "_id", Value: sha1}})
	if err != nil {
		return fmt.Errorf("remover imagem %s: %w", sha1, err)
	}
	return nil
}

// FindImage busca uma imagem pelo sha1.
func (m *MongoRepo) FindImage(ctx context.Context, sha1 string) (*Image, error) {
	var img Image
	err := m.db.Collection("images").FindOne(ctx, bson.D{{Key: "_id", Value: sha1}}).Decode(&img)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar imagem %s: %w", sha1, err)
	}
	img.fromBSON()
	return &img, nil
}

// ReplaceImage grava o documento inteiro por cima do atual, MAS só se ele
// ainda estiver como foi lido (updated_at igual a prevUpdatedAt).
//
// Isso é "optimistic locking": em vez de travar a imagem enquanto alguém
// edita, deixamos todo mundo ler, e na hora de gravar conferimos se ninguém
// gravou no meio. Se gravou, devolvemos ErrConflict e o front recarrega.
func (m *MongoRepo) ReplaceImage(ctx context.Context, img *Image, prevUpdatedAt time.Time) error {
	filter := bson.D{{Key: "_id", Value: img.SHA1}, {Key: "updated_at", Value: prevUpdatedAt}}
	res, err := m.db.Collection("images").ReplaceOne(ctx, filter, img)
	if err != nil {
		return fmt.Errorf("atualizar imagem %s: %w", img.SHA1, err)
	}
	if res.MatchedCount == 0 {
		// Não achou com aquele updated_at: ou a imagem sumiu, ou mudou.
		if _, err := m.FindImage(ctx, img.SHA1); errors.Is(err, platform.ErrNotFound) {
			return platform.ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

func (f ImageFilter) toBSON() bson.D {
	filter := bson.D{}
	add := func(field string, value any) {
		if value != nil && value != "" {
			filter = append(filter, bson.E{Key: field, Value: value})
		}
	}
	if len(f.IDs) > 0 {
		add("_id", bson.D{{Key: "$in", Value: f.IDs}})
	}
	switch {
	case len(f.Packages) == 0:
		add("package", f.Package)
	case f.Package == "":
		add("package", bson.D{{Key: "$in", Value: f.Packages}})
	default: // os dois: uma chave só (duas "package" no filtro, uma apagaria a outra)
		add("package", bson.D{{Key: "$eq", Value: f.Package}, {Key: "$in", Value: f.Packages}})
	}
	add("status", f.Status)
	for _, k := range sortedKeys(f.Fields) {
		add("fields."+k, f.Fields[k]) // numa lista, o Mongo casa se ela contém o valor
	}

	// Os filtros de objeto usam $elemMatch: todas as condições precisam valer
	// para o MESMO objeto. Sem isso, "caminhão branco" traria uma imagem
	// com um caminhão preto e um carro branco.
	elem := func(typ string, fields map[string]any) bson.E {
		obj := bson.D{}
		if typ != "" {
			obj = append(obj, bson.E{Key: "type", Value: typ})
		}
		for _, k := range sortedKeys(fields) {
			obj = append(obj, bson.E{Key: "fields." + k, Value: fields[k]})
		}
		return bson.E{Key: "objects", Value: bson.D{{Key: "$elemMatch", Value: obj}}}
	}
	var objs []bson.D
	if len(f.Object) > 0 {
		objs = append(objs, bson.D{elem("", f.Object)})
	}
	for _, o := range f.Objects {
		objs = append(objs, bson.D{elem(o.Type, o.Fields)})
	}
	switch len(objs) {
	case 0:
	case 1:
		filter = append(filter, objs[0]...)
	default: // um $elemMatch por objeto: no mesmo nível, um sobrescreveria o outro
		and := bson.A{}
		for _, o := range objs {
			and = append(and, o)
		}
		filter = append(filter, bson.E{Key: "$and", Value: and})
	}
	return filter
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// fromBSON troca os tipos que o Mongo devolve dentro de fields/objects pelos
// mesmos que o Check do perfil produz ([]string, time.Time, float64). Assim
// a imagem lida do banco e a recém-anotada são comparáveis (o histórico não
// acusa mudança onde não houve) e o JSON sai igual.
func (img *Image) fromBSON() {
	img.ensureValues()
	for k, v := range img.Fields {
		img.Fields[k] = bsonValue(v)
	}
	for i := range img.Objects {
		for k, v := range img.Objects[i].Fields {
			img.Objects[i].Fields[k] = bsonValue(v)
		}
	}
}

func bsonValue(v any) any {
	switch x := v.(type) {
	case bson.A:
		strs := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				out := make([]any, len(x))
				for i, e := range x {
					out[i] = bsonValue(e)
				}
				return out
			}
			strs = append(strs, s)
		}
		return strs
	case bson.DateTime:
		return x.Time().UTC()
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	}
	return v
}

// ListImages devolve uma página de resultados e o total que bate com o filtro.
// Ordem: a mesma da fila de trabalho (queued_at), mais antigas primeiro.
func (m *MongoRepo) ListImages(ctx context.Context, f ImageFilter) ([]Image, int64, error) {
	coll := m.db.Collection("images")
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
	for i := range items {
		items[i].fromBSON()
	}
	return items, total, nil
}

// InsertHistory grava várias entradas de uma vez (nada a fazer se vazio).
func (m *MongoRepo) InsertHistory(ctx context.Context, entries []HistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if _, err := m.db.Collection("image_history").InsertMany(ctx, entries); err != nil {
		return fmt.Errorf("gravar histórico: %w", err)
	}
	return nil
}

// ExistingImages recebe uma lista de sha1 e devolve quais já existem,
// com pacote e status de cada um. Uma consulta só ($in), não uma por hash.
func (m *MongoRepo) ExistingImages(ctx context.Context, ids []string) (map[string]ExistingImage, error) {
	filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}
	opts := options.Find().SetProjection(bson.D{{Key: "status", Value: 1}, {Key: "package", Value: 1}}) // _id vem sempre

	cur, err := m.db.Collection("images").Find(ctx, filter, opts)
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

// unsetClaim remove quem está com a imagem.
var unsetClaim = bson.D{{Key: "$unset", Value: bson.D{
	{Key: "claimed_by", Value: ""},
	{Key: "claimed_until", Value: ""},
}}}

// ClaimNext entrega a próxima imagem pendente do pacote para o usuário.
//
//  1. Se skip != "", a imagem pulada vai para o fim da fila (queued_at = agora).
//  2. Libera as outras imagens que o usuário estava segurando neste pacote:
//     cada pessoa trabalha em UMA imagem por vez.
//  3. Pega a imagem pendente livre mais antiga da fila e marca como dele
//     até now+lease.
//
// O passo 3 é um FindOneAndUpdate: o Mongo acha E marca numa operação só,
// atômica. Duas pessoas clicando "próxima" no mesmo milissegundo nunca
// recebem a mesma imagem.
//
// Se o usuário já estava com uma imagem (e não pulou), ela é liberada no
// passo 2 e, como é a mais antiga livre, volta para ele no passo 3. Ou seja:
// recarregar a página devolve a mesma imagem.
//
// Fila vazia: ErrNotFound.
func (m *MongoRepo) ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*Image, error) {
	coll := m.db.Collection("images")

	if skip != "" {
		_, err := coll.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: skip}, {Key: "package", Value: pkg}, {Key: "claimed_by", Value: user}},
			bson.D{
				{Key: "$set", Value: bson.D{{Key: "queued_at", Value: now}}},
				{Key: "$unset", Value: bson.D{{Key: "claimed_by", Value: ""}, {Key: "claimed_until", Value: ""}}},
			})
		if err != nil {
			return nil, fmt.Errorf("pular imagem %s: %w", skip, err)
		}
	}

	_, err := coll.UpdateMany(ctx,
		bson.D{{Key: "package", Value: pkg}, {Key: "status", Value: StatusPending}, {Key: "claimed_by", Value: user}},
		unsetClaim)
	if err != nil {
		return nil, fmt.Errorf("liberar imagens de %s: %w", user, err)
	}

	free := bson.D{
		{Key: "package", Value: pkg},
		{Key: "status", Value: StatusPending},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "claimed_until", Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: "claimed_until", Value: bson.D{{Key: "$lte", Value: now}}}}, // lease vencido
		}},
	}
	claim := bson.D{{Key: "$set", Value: bson.D{
		{Key: "claimed_by", Value: user},
		{Key: "claimed_until", Value: now.Add(lease)},
	}}}
	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "queued_at", Value: 1}, {Key: "_id", Value: 1}}).
		SetReturnDocument(options.After) // devolve o documento JÁ marcado

	// Quem pulou não pode receber de volta a imagem que acabou de pular.
	// Não dá para confiar só no queued_at: duas ações no mesmo milissegundo
	// (o relógio do Windows tem resolução de alguns ms) empatam, e o
	// desempate pelo _id podia devolver justo a imagem pulada.
	// Se ela for a única pendente livre, aí sim ela volta.
	if skip != "" {
		notSkipped := append(bson.D{{Key: "_id", Value: bson.D{{Key: "$ne", Value: skip}}}}, free...)
		img, err := claimOne(ctx, coll, notSkipped, claim, opts)
		if !errors.Is(err, platform.ErrNotFound) {
			return img, err
		}
	}
	return claimOne(ctx, coll, free, claim, opts)
}

func claimOne(ctx context.Context, coll *mongo.Collection, filter, update bson.D, opts *options.FindOneAndUpdateOptionsBuilder) (*Image, error) {
	var img Image
	err := coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&img)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pegar próxima imagem: %w", err)
	}
	img.fromBSON()
	return &img, nil
}

// ReleaseClaim devolve a imagem para a fila (ex: a pessoa fechou a aba).
// Só quem está com a imagem pode liberar.
func (m *MongoRepo) ReleaseClaim(ctx context.Context, sha1, user string) error {
	_, err := m.db.Collection("images").UpdateOne(ctx,
		bson.D{{Key: "_id", Value: sha1}, {Key: "claimed_by", Value: user}},
		unsetClaim)
	if err != nil {
		return fmt.Errorf("liberar imagem %s: %w", sha1, err)
	}
	return nil
}

// ---------- o que os outros módulos pedem (via Service) ----------

// PackageProgress conta as imagens de cada pacote por status, numa
// agregação só, sobre os pacotes pedidos:
//
//	{ _id: { package: "p1", status: "pending" },  n: 12, claimed: 2 }
//	{ _id: { package: "p1", status: "verified" }, n: 30, claimed: 0 }
//
// Pacote sem imagens não aparece no mapa (quem chama usa zeros).
func (m *MongoRepo) PackageProgress(ctx context.Context, names []string) (map[string]catalog.Progress, error) {
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
	agg, err := m.db.Collection("images").Aggregate(ctx, pipeline)
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
func (m *MongoRepo) RenamePackageImages(ctx context.Context, oldName, newName string) (int64, error) {
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
	res, err := m.db.Collection("images").UpdateMany(ctx, bson.D{{Key: "package", Value: oldName}}, pipeline)
	if err != nil {
		return 0, fmt.Errorf("renomear o pacote nas imagens: %w", err)
	}
	return res.ModifiedCount, nil
}

// DeletePackageImages apaga todas as imagens do pacote e o histórico delas.
// Os arquivos no disco ficam por conta de quem chama. Devolve quantas
// imagens foram apagadas.
func (m *MongoRepo) DeletePackageImages(ctx context.Context, name string) (int64, error) {
	images := m.db.Collection("images")
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
		if _, err := m.db.Collection("image_history").DeleteMany(ctx,
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
func (m *MongoRepo) CountUsing(ctx context.Context, reg catalog.Registry, name string) (int64, error) {
	n, err := m.db.Collection("images").CountDocuments(ctx, bson.D{{Key: "fields." + reg.ImageField(), Value: name}})
	if err != nil {
		return 0, fmt.Errorf("contar imagens com %s %s: %w", reg.ImageField(), name, err)
	}
	return n, nil
}

// RenameOnPending troca o nome nas imagens PENDENTES, de uma vez só.
// Nas pendentes o nome não está no arquivo (que ainda se chama <sha1>.jpg),
// então basta o banco. O updated_at muda junto: quem estiver com a imagem
// aberta recebe 409 ao salvar, e recarrega com o nome novo.
func (m *MongoRepo) RenameOnPending(ctx context.Context, reg catalog.Registry, oldName, newName string, now time.Time) (int64, error) {
	field := "fields." + reg.ImageField()
	res, err := m.db.Collection("images").UpdateMany(ctx,
		bson.D{{Key: field, Value: oldName}, {Key: "status", Value: StatusPending}},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: newName}, {Key: "updated_at", Value: now}}}})
	if err != nil {
		return 0, fmt.Errorf("renomear %s nas pendentes: %w", field, err)
	}
	return res.ModifiedCount, nil
}

// ---------- o lado das imagens do módulo boxes ----------

// VerifiedIDs: quais destes sha1 são de imagens verificadas.
func (m *MongoRepo) VerifiedIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	const batch = 1000
	for start := 0; start < len(ids); start += batch {
		end := min(start+batch, len(ids))
		cur, err := m.db.Collection("images").Find(ctx, bson.D{
			{Key: "_id", Value: bson.D{{Key: "$in", Value: ids[start:end]}}},
			{Key: "status", Value: StatusVerified},
		}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return nil, fmt.Errorf("imagens verificadas: %w", err)
		}
		var docs []struct {
			ID string `bson:"_id"`
		}
		if err := cur.All(ctx, &docs); err != nil {
			return nil, fmt.Errorf("ler imagens verificadas: %w", err)
		}
		for _, d := range docs {
			out[d.ID] = true
		}
	}
	return out, nil
}

// FindImagesByKeys busca as imagens de um pacote (pkg "" = de todos) pela
// chave do import:
// by = "original_name" (nome do arquivo antes do upload) ou "sha1".
// Pode voltar mais de uma imagem por nome original (duas pastas diferentes
// com um "img001.jpg" cada): quem chama decide o que fazer.
// Consulta em lotes de 1000 chaves, para um CSV de 50 mil linhas não virar
// uma consulta gigante.
func (m *MongoRepo) FindImagesByKeys(ctx context.Context, pkg, by string, keys []string) ([]Image, error) {
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
		cur, err := m.db.Collection("images").Find(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("buscar imagens do import: %w", err)
		}
		var imgs []Image
		if err := cur.All(ctx, &imgs); err != nil {
			return nil, fmt.Errorf("ler imagens do import: %w", err)
		}
		for i := range imgs {
			imgs[i].fromBSON()
		}
		out = append(out, imgs...)
	}
	return out, nil
}

// Stats conta tudo num aggregate só: um ramo do $facet por contagem. Os nomes
// dos ramos são posições (f0, o0, o0_1...), porque a chave de um campo pode
// ter caracteres que um nome de ramo não aceita.
//
// $unwind num valor que não é lista trata ele como uma lista de um, então o
// mesmo ramo conta choice, bool e multi; imagem sem o campo não entra.
func (m *MongoRepo) Stats(ctx context.Context, f ImageFilter, spec StatsSpec) (*Stats, error) {
	group := func(by string) bson.D {
		return bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: by}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}
	}
	unwind := func(path string) bson.D { return bson.D{{Key: "$unwind", Value: "$" + path}} }
	facet := bson.D{
		{Key: "total", Value: bson.A{bson.D{{Key: "$count", Value: "n"}}}},
		{Key: "packages", Value: bson.A{group("$package")}},
		{Key: "status", Value: bson.A{group("$status")}},
	}
	// com valor: nem ausente, nem null, nem "", nem lista vazia (o resto é o
	// "Não informado", que sai por diferença do total)
	present := func(path string) bson.D {
		return bson.D{{Key: "$match", Value: bson.D{{Key: path, Value: bson.D{{Key: "$nin", Value: bson.A{nil, "", bson.A{}}}}}}}}
	}
	count := bson.D{{Key: "$count", Value: "n"}}
	for i, k := range spec.Fields {
		facet = append(facet,
			bson.E{Key: fmt.Sprintf("f%d", i), Value: bson.A{unwind("fields." + k), group("$fields." + k)}},
			bson.E{Key: fmt.Sprintf("f%dp", i), Value: bson.A{present("fields." + k), count}})
	}
	types := sortedKeys(spec.Objects)
	for i, t := range types {
		ofType := bson.D{{Key: "$match", Value: bson.D{{Key: "objects.type", Value: t}}}}
		// por imagem (quantos do tipo ela tem), depois o total: imagens e objetos
		facet = append(facet, bson.E{Key: fmt.Sprintf("o%d", i), Value: bson.A{
			unwind("objects"), ofType,
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "images", Value: bson.D{{Key: "$sum", Value: 1}}}, {Key: "count", Value: bson.D{{Key: "$sum", Value: "$n"}}}}}},
		}})
		for j, k := range spec.Objects[t] {
			facet = append(facet,
				bson.E{Key: fmt.Sprintf("o%d_%d", i, j), Value: bson.A{
					unwind("objects"), ofType, unwind("objects.fields." + k), group("$objects.fields." + k),
				}},
				bson.E{Key: fmt.Sprintf("o%d_%dp", i, j), Value: bson.A{unwind("objects"), ofType, present("objects.fields." + k), count}})
		}
	}
	pipeline := bson.A{bson.D{{Key: "$match", Value: f.toBSON()}}, bson.D{{Key: "$facet", Value: facet}}}

	cur, err := m.db.Collection("images").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	st := emptyStats(spec)
	if len(out) == 0 {
		return st, nil
	}
	res := out[0]
	// Os documentos de dentro do resultado vêm como bson.D no driver v2 (só o
	// de fora é bson.M, porque foi o pedido): aceita os dois.
	rows := func(name string) []bson.M {
		arr, _ := res[name].(bson.A)
		list := make([]bson.M, 0, len(arr))
		for _, e := range arr {
			switch d := e.(type) {
			case bson.M:
				list = append(list, d)
			case bson.D:
				m := bson.M{}
				for _, kv := range d {
					m[kv.Key] = kv.Value
				}
				list = append(list, m)
			}
		}
		return list
	}
	fill := func(c Counts, name string) {
		for _, r := range rows(name) {
			if r["_id"] != nil {
				c[countKey(bsonValue(r["_id"]))] += toInt(r["n"])
			}
		}
	}
	if t := rows("total"); len(t) > 0 {
		st.Total = toInt(t[0]["n"])
	}
	fill(st.Packages, "packages")
	fill(st.Status, "status")
	counted := func(name string) int64 {
		if r := rows(name); len(r) > 0 {
			return toInt(r[0]["n"])
		}
		return 0
	}
	for i, k := range spec.Fields {
		fill(st.Fields[k], fmt.Sprintf("f%d", i))
		st.Missing[k] = st.Total - counted(fmt.Sprintf("f%dp", i))
	}
	for i, t := range types {
		o := st.Objects[t]
		if r := rows(fmt.Sprintf("o%d", i)); len(r) > 0 {
			o.Images, o.Count = toInt(r[0]["images"]), toInt(r[0]["count"])
		}
		for j, k := range spec.Objects[t] {
			fill(o.Fields[k], fmt.Sprintf("o%d_%d", i, j))
			o.Missing[k] = o.Count - counted(fmt.Sprintf("o%d_%dp", i, j))
		}
	}
	return st, nil
}

// toInt: as contagens do aggregate vêm como int32 ou int64.
func toInt(v any) int64 {
	switch x := v.(type) {
	case int32:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}

func (m *MongoRepo) MatchingIDs(ctx context.Context, f ImageFilter) ([]string, error) {
	cur, err := m.db.Collection("images").Find(ctx, f.toBSON(),
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids, nil
}

// NormalizeValue: um valor lido do Mongo no tipo que o resto usa (lista de
// texto = []string, número = float64, data = time.Time). O módulo versions
// usa para os itens congelados.
func NormalizeValue(v any) any { return bsonValue(v) }

// ---------- lixeira (images_trash) ----------

func (m *MongoRepo) PutTrash(ctx context.Context, t *Trash) error {
	_, err := m.db.Collection("images_trash").ReplaceOne(ctx, bson.D{{Key: "_id", Value: t.SHA1}}, t, options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("guardar %s na lixeira: %w", t.SHA1, err)
	}
	return nil
}

func (m *MongoRepo) FindTrash(ctx context.Context, sha1 string) (*Trash, error) {
	var t Trash
	err := m.db.Collection("images_trash").FindOne(ctx, bson.D{{Key: "_id", Value: sha1}}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar %s na lixeira: %w", sha1, err)
	}
	t.fromBSON()
	return &t, nil
}

func (m *MongoRepo) TrashBefore(ctx context.Context, before time.Time, limit int) ([]Trash, error) {
	cur, err := m.db.Collection("images_trash").Find(ctx,
		bson.D{{Key: "deleted_at", Value: bson.D{{Key: "$lt", Value: before}}}},
		options.Find().SetLimit(int64(limit)).SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "ext", Value: 1}, {Key: "deleted_at", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar a lixeira: %w", err)
	}
	var out []Trash
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("listar a lixeira: %w", err)
	}
	return out, nil
}

func (m *MongoRepo) DeleteTrash(ctx context.Context, sha1 string) error {
	if _, err := m.db.Collection("images_trash").DeleteOne(ctx, bson.D{{Key: "_id", Value: sha1}}); err != nil {
		return fmt.Errorf("tirar %s da lixeira: %w", sha1, err)
	}
	return nil
}
