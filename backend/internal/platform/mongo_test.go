package platform

import (
	"context"
	"testing"
	"time"
)

func TestConnectMongoInvalido(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := ConnectMongo(ctx, "http://nao-e-mongo"); err == nil {
		t.Error("esquema inválido deveria falhar no Connect")
	}
	if _, err := ConnectMongo(ctx, "mongodb://localhost:1"); err == nil {
		t.Error("porta sem Mongo deveria falhar no Ping")
	}
}
