package redislock

import (
	"context"
	_ "embed"
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

var (

	//go:embed unlock.lua
	luaUnlock string
	//go:embed refresh.lua
	refreshLock string

	ErrLockNotHold       = errors.New("未持有锁")
	ErrFailedPreemptLock = errors.New("加锁失败")
)

type RedisLock struct {
	client     redis.Cmdable
	key        string
	value      string
	expiration time.Duration
	unlockCh   chan struct{}
}

func NewRedisLock(client redis.Cmdable, key string) *RedisLock {
	return &RedisLock{
		client:   client,
		key:      key,
		unlockCh: make(chan struct{}, 1),
	}
}

func (l *RedisLock) TryLock(ctx context.Context, expiration time.Duration) error {
	l.value = uuid.NewString()
	l.expiration = expiration
	res, err := l.client.SetNX(ctx, l.key, l.value, l.expiration).Result()
	if err != nil {
		return err
	}
	if !res {
		return ErrFailedPreemptLock
	}
	return nil
}

func (l *RedisLock) UnLock(ctx context.Context) error {
	defer func() {
		l.unlockCh <- struct{}{}
	}()
	res, err := l.client.Eval(ctx, luaUnlock, []string{l.key}, l.value).Int64()
	if err != nil {
		if err == redis.Nil {
			return ErrLockNotHold
		}
		return err
	}
	if res == 0 {
		return ErrLockNotHold
	}
	return nil
}

func (l *RedisLock) Refresh(ctx context.Context) error {
	res, err := l.client.Eval(ctx, refreshLock, []string{l.key}, l.value, l.expiration.Milliseconds()).Int64()
	if err != nil {
		if err == redis.Nil {
			return ErrLockNotHold
		}
		return err
	}
	if res != 1 {
		return ErrLockNotHold
	}
	return nil
}

func (l *RedisLock) AutoRefresh(interval time.Duration, timeout time.Duration) error {
	retryCh := make(chan struct{}, 1)
	defer func() {
		close(retryCh)
	}()
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-retryCh:
			ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
			err := l.Refresh(ctx)
			cancelFunc()
			if err == context.DeadlineExceeded {
				// 超时重试
				retryCh <- struct{}{}
				continue
			}
			if err != nil {
				return err
			}
		case <-ticker.C:
			ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
			err := l.Refresh(ctx)
			cancelFunc()
			if err == context.DeadlineExceeded {
				// 超时重试
				retryCh <- struct{}{}
				continue
			}
			if err != nil {
				return err
			}
		case <-l.unlockCh:
			return nil
		}
	}
}
