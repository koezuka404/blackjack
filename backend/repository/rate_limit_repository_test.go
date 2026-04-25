package repository

import (
	"context"
	"testing"
)

func TestNewRedisTokenBucketRepository(t *testing.T) {
	repo := NewRedisTokenBucketRepository(nil, 20, 5)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestRedisTokenBucketRepository_Allow_NilClient(t *testing.T) {
	repo := &RedisTokenBucketRepository{}
	_, _, _, err := repo.Allow(context.Background(), "key", 1, 1, 1, 1)
	if err == nil {
		t.Fatal("expected error for nil redis client")
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{name: "int64", in: int64(12), want: 12, ok: true},
		{name: "float64", in: float64(12), want: 12, ok: true},
		{name: "string", in: "12", want: 12, ok: true},
		{name: "invalid", in: true, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toInt64(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
			if tt.ok && got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{name: "float64", in: float64(12.5), want: 12.5, ok: true},
		{name: "int64", in: int64(12), want: 12, ok: true},
		{name: "string", in: "12.5", want: 12.5, ok: true},
		{name: "invalid", in: true, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toFloat64(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
			if tt.ok && got != tt.want {
				t.Fatalf("got %f want %f", got, tt.want)
			}
		})
	}
}

