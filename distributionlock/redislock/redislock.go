package redislock

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"time"
)

var (

	//go:embed unlock.lua
	luaUnlock string
	//go:embed refresh.lua
	refreshLock string
	//go:embed lock.lua
	luaLock string

	ErrLockNotHold       = errors.New("未持有锁")
	ErrFailedPreemptLock = errors.New("加锁失败")
)

type RedisLock struct {
	client     redis.Cmdable
	key        string
	value      string
	expiration time.Duration
	unlockCh   chan struct{}
	sg         singleflight.Group
}

func NewRedisLock(client redis.Cmdable, key string) *RedisLock {
	return &RedisLock{
		client:   client,
		key:      key,
		unlockCh: make(chan struct{}, 1),
	}
}

func (l *RedisLock) Lock(ctx context.Context, expiration time.Duration, retry RetryStrategy, timeout time.Duration) error {
	l.value = uuid.NewString()
	l.expiration = expiration
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		lctx, cancelFunc := context.WithTimeout(ctx, timeout)
		res, err := l.client.Eval(lctx, luaLock, []string{l.key}, l.value, l.expiration.Seconds()).Result()
		cancelFunc()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if res == "OK" {
			return nil
		}
		interval, ok := retry.Next()
		if !ok {
			if err != nil {
				err = fmt.Errorf("最后一次重试错误：%w", err)
			} else {
				err = fmt.Errorf("锁被人持有：%w", ErrFailedPreemptLock)
			}
			return fmt.Errorf("RedisLocl:重试机会耗尽,%w", err)
		}
		if timer == nil {
			timer = time.NewTimer(interval)
		} else {
			timer.Reset(interval)
		}
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (l *RedisLock) SingleflightLock(ctx context.Context, expiration time.Duration, retry RetryStrategy, timeout time.Duration) error {
	for {
		flag := false
		result := l.sg.DoChan(l.key, func() (interface{}, error) {
			flag = true
			return l.Lock(ctx, expiration, retry, timeout), nil
		})
		select {
		case res := <-result:
			if flag {
				l.sg.Forget(l.key)
				if res.Err != nil {
					return res.Err
				}
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
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
		close(l.unlockCh)
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
