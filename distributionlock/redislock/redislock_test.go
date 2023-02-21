package redislock

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/iconnor-code/gopkg/distributionlock/redislock/mocks"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRedisLock_TryLock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	type fields struct {
		client     func() redis.Cmdable
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
	}{
		{
			name: "成功获取锁",
			fields: fields{
				client: func() redis.Cmdable {
					rdb := mocks.NewMockCmdable(ctrl)
					res := redis.NewBoolResult(true, nil)
					rdb.EXPECT().
						SetNX(gomock.Any(), "lock-key", gomock.Any(), time.Minute).
						Return(res)
					return rdb
				},
				key: "lock-key",
			},
			args: args{
				ctx:        context.Background(),
				expiration: time.Minute,
			},
			wantErr: nil,
		},
		{
			name: "获取锁失败",
			fields: fields{
				client: func() redis.Cmdable {
					rdb := mocks.NewMockCmdable(ctrl)
					res := redis.NewBoolResult(false, nil)
					rdb.EXPECT().
						SetNX(gomock.Any(), "lock-key", gomock.Any(), time.Minute).
						Return(res)
					return rdb
				},
				key: "lock-key",
			},
			args: args{
				ctx:        context.Background(),
				expiration: time.Minute,
			},
			wantErr: ErrFailedPreemptLock,
		},
		{
			name: "返回其它错误",
			fields: fields{
				client: func() redis.Cmdable {
					rdb := mocks.NewMockCmdable(ctrl)
					res := redis.NewBoolResult(false, errors.New("其它错误"))
					rdb.EXPECT().
						SetNX(gomock.Any(), "lock-key", gomock.Any(), time.Minute).
						Return(res)
					return rdb
				},
				key: "lock-key",
			},
			args: args{
				ctx:        context.Background(),
				expiration: time.Minute,
			},
			wantErr: errors.New("其它错误"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &RedisLock{
				client: tt.fields.client(),
				key:    tt.fields.key,
			}
			err := l.TryLock(tt.args.ctx, tt.args.expiration)
			assert.NotEmpty(t, l.value)
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.args.expiration, l.expiration)
		})
	}
}
