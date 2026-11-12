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
func (s *Store) ClaimNext(ctx context.Context, pkg, user, skip string, lease time.Duration, now time.Time) (*Image, error) {
	coll := s.db.Collection("images")

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

	var img Image
	err = coll.FindOneAndUpdate(ctx, free, claim, opts).Decode(&img)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pegar próxima imagem de %s: %w", pkg, err)
	}
	return &img, nil
}

// ReleaseClaim devolve a imagem para a fila (ex: a pessoa fechou a aba).
// Só quem está com a imagem pode liberar.
func (s *Store) ReleaseClaim(ctx context.Context, sha1, user string) error {
	_, err := s.db.Collection("images").UpdateOne(ctx,
		bson.D{{Key: "_id", Value: sha1}, {Key: "claimed_by", Value: user}},
		unsetClaim)
	if err != nil {
		return fmt.Errorf("liberar imagem %s: %w", sha1, err)
	}
	return nil
}
