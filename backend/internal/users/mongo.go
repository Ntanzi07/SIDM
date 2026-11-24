package users

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"SIDM/internal/platform"
)

// MongoRepo são as coleções users e sessions.
type MongoRepo struct{ db *mongo.Database }

func NewMongoRepo(db *mongo.Database) *MongoRepo { return &MongoRepo{db: db} }

func (m *MongoRepo) users() *mongo.Collection    { return m.db.Collection("users") }
func (m *MongoRepo) sessions() *mongo.Collection { return m.db.Collection("sessions") }

// EnsureIndexes: as sessões somem sozinhas depois de expirar (TTL) e são
// buscadas por pessoa (troca de senha). Idempotente.
func (m *MongoRepo) EnsureIndexes(ctx context.Context) error {
	if _, err := m.sessions().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "username", Value: 1}}},
	}); err != nil {
		return fmt.Errorf("índices de sessions: %w", err)
	}
	return nil
}

func (m *MongoRepo) CreateUser(ctx context.Context, u *User) error {
	_, err := m.users().InsertOne(ctx, u)
	if mongo.IsDuplicateKeyError(err) {
		return platform.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("criar usuário %s: %w", u.Username, err)
	}
	return nil
}

func (m *MongoRepo) FindUser(ctx context.Context, username string) (*User, error) {
	var u User
	err := m.users().FindOne(ctx, bson.D{{Key: "_id", Value: username}}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar usuário %s: %w", username, err)
	}
	return &u, nil
}

func (m *MongoRepo) ListUsers(ctx context.Context) ([]User, error) {
	cur, err := m.users().Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("listar usuários: %w", err)
	}
	list := []User{}
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("listar usuários: %w", err)
	}
	return list, nil
}

func (m *MongoRepo) ReplaceUser(ctx context.Context, u *User) error {
	res, err := m.users().ReplaceOne(ctx, bson.D{{Key: "_id", Value: u.Username}}, u)
	if err != nil {
		return fmt.Errorf("gravar usuário %s: %w", u.Username, err)
	}
	if res.MatchedCount == 0 {
		return platform.ErrNotFound
	}
	return nil
}

func (m *MongoRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	n, err := m.users().CountDocuments(ctx, bson.D{{Key: "role", Value: RoleAdmin}, {Key: "status", Value: StatusActive}})
	if err != nil {
		return 0, fmt.Errorf("contar admins: %w", err)
	}
	return n, nil
}

func (m *MongoRepo) CreateSession(ctx context.Context, s *Session) error {
	if _, err := m.sessions().InsertOne(ctx, s); err != nil {
		return fmt.Errorf("criar sessão: %w", err)
	}
	return nil
}

func (m *MongoRepo) FindSession(ctx context.Context, id string) (*Session, error) {
	var s Session
	err := m.sessions().FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, platform.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscar sessão: %w", err)
	}
	return &s, nil
}

func (m *MongoRepo) DeleteSession(ctx context.Context, id string) error {
	if _, err := m.sessions().DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}); err != nil {
		return fmt.Errorf("apagar sessão: %w", err)
	}
	return nil
}

func (m *MongoRepo) DeleteSessions(ctx context.Context, username string) error {
	if _, err := m.sessions().DeleteMany(ctx, bson.D{{Key: "username", Value: username}}); err != nil {
		return fmt.Errorf("apagar as sessões de %s: %w", username, err)
	}
	return nil
}
