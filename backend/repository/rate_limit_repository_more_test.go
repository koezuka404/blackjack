package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisTokenBucketRepository_Allow_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	repo := NewRedisTokenBucketRepository(rdb, 20, 5)
	ok, tokens, retry, err := repo.Allow(context.Background(), "k1", 5, 20, 1, time.Now().UTC().UnixMilli())
	if err != nil {
		t.Fatalf("allow should succeed: %v", err)
	}
	if !ok || tokens < 0 || retry != 0 {
		t.Fatalf("unexpected allow result: ok=%v tokens=%f retry=%d", ok, tokens, retry)
	}
}

func TestRedisTokenBucketRepository_Allow_LuaResultErrors(t *testing.T) {
	prev := tokenBucketScript
	t.Cleanup(func() { tokenBucketScript = prev })

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	repo := &RedisTokenBucketRepository{rdb: rdb}

	tokenBucketScript = redis.NewScript(`return "bad"`)
	_, _, _, err := repo.Allow(context.Background(), "k1", 1, 1, 1, time.Now().UTC().UnixMilli())
	if err == nil {
		t.Fatal("expected unexpected lua result error")
	}

	tokenBucketScript = redis.NewScript(`return {"x","1","1"}`)
	_, _, _, err = repo.Allow(context.Background(), "k1", 1, 1, 1, time.Now().UTC().UnixMilli())
	if err == nil {
		t.Fatal("expected parse allowed error")
	}

	tokenBucketScript = redis.NewScript(`return {1,{"x"},"1"}`)
	_, _, _, err = repo.Allow(context.Background(), "k1", 1, 1, 1, time.Now().UTC().UnixMilli())
	if err == nil {
		t.Fatal("expected parse tokens error")
	}

	tokenBucketScript = redis.NewScript(`return {1,"1",{"x"}}`)
	_, _, _, err = repo.Allow(context.Background(), "k1", 1, 1, 1, time.Now().UTC().UnixMilli())
	if err == nil {
		t.Fatal("expected parse retry error")
	}
}

func TestRedisTokenBucketRepository_Allow_RedisError(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:59999"})
	defer rdb.Close()
	repo := &RedisTokenBucketRepository{rdb: rdb}
	_, _, _, err := repo.Allow(context.Background(), "k", 1, 1, 1, time.Now().UTC().UnixMilli())
	if err == nil {
		t.Fatal("expected redis dial/eval error")
	}
}
