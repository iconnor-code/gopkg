//go:build e2e

package redislock

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRedisLock_TryLock_e2e(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:63790",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	type fields struct {
		client     redis.Cmdable
		key        string
		value      string
		expiration time.Duration
	}
	type args struct {
		ctx        context.Context
		expiration time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
		before  func()
		after   func()
	}{
		{
			name: "成功获取锁",
			fields: fields{
				client: rdb,
				key:    "success-key",
			},
			args: args{
				ctx:        context.Background(),
				expiration: time.Minute,
			},
			wantErr: nil,
			before:  func() {},
			after: func() {
				res, err := rdb.Del(context.Background(), "success-key").Uint64()
				require.NoError(t, err)
				require.Equal(t, uint64(1), res)
			},
		},
		{
			name: "获取锁失败",
			fields: fields{
				client: rdb,
				key:    "fail-key",
			},
			args: args{
				ctx:        context.Background(),
				expiration: time.Minute,
			},
			wantErr: ErrFailedPreemptLock,
			before: func() {
				res, err := rdb.Set(context.Background(), "fail-key", "123", time.Minute).Result()
				require.NoError(t, err)
				require.Equal(t, "OK", res)
			},
			after: func() {
				res, err := rdb.Get(context.Background(), "fail-key").Result()
				require.NoError(t, err)
				require.Equal(t, "123", res)

				count, err := rdb.Del(context.Background(), "fail-key").Uint64()
				require.NoError(t, err)
				require.Equal(t, uint64(1), count)
			},
		},
	}
	for _, tt := range tests {
		tt.before()

		t.Run(tt.name, func(t *testing.T) {
			l := &RedisLock{
				client: tt.fields.client,
				key:    tt.fields.key,
			}
			err := l.TryLock(tt.args.ctx, tt.args.expiration)
			assert.NotEmpty(t, l.value)
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.args.expiration, l.expiration)

			if tt.fields.key == "success-key" {
				v, err := rdb.Get(tt.args.ctx, tt.fields.key).Result()
				assert.NoError(t, err)
				assert.Equal(t, l.value, v)
			}

			tt.after()
		})
	}
}
