package redis_lock

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
	ErrFailedToPreemptLock = errors.New("redis-lock: 抢锁失败")
	ErrLockNotHold         = errors.New("redis-lock: 你没有持有锁")
	
	//go:embed lua/unlock.lua
	luaUnlock string
	
	//go:embed lua/refresh.lua
	luaRefresh string
	
	//go:embed lua/lock.lua
	luaLock string
)

type Client struct {
	client redis.Cmdable
	g singleflight.Group
}

func NewClient(client redis.Cmdable) *Client {
	return &Client{
		client: client,
	}
}

func (c *Client) SingleflightLock(ctx context.Context,
	key string, expiration time.Duration,
	timeout time.Duration, retry RetryStrategy) (*Lock, error) {
	for {
		flag := false
		resCh := c.g.DoChan(key, func() (interface{}, error) {
			flag = true
			return c.Lock(ctx, key, expiration, timeout, retry)
		})
		select {
		case res := <- resCh:
			if flag {
				c.g.Forget(key)
				if res.Err != nil {
					return nil, res.Err
				}
				return res.Val.(*Lock), nil
			}
		case <- ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) Lock(ctx context.Context, 
	key string, expiration time.Duration, 
	timeout time.Duration, retry RetryStrategy) (*Lock, error) {
	
	var timer *time.Timer
	val := uuid.New().String()
	for {
		// 进行超时重试
		lctx, cancel := context.WithTimeout(ctx, timeout)
		res, err := c.client.Eval(lctx, luaLock, []string{key}, val, expiration.Seconds()).Result()
		cancel()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		
		if res == "OK" {
			return &Lock{
				client:     c.client,
				key:        key,
				val:      val,
				expiration: expiration,
				unlockCh: make(chan struct{}, 1),
			}, nil
		}
		
		interval, ok := retry.Next()
		if !ok {
			return nil, fmt.Errorf("redis-lock: 超出重试限制, %w", ErrFailedToPreemptLock)
		}
		if timer == nil {
			timer = time.NewTimer(interval) 
		} else {
			timer.Reset(interval)
		}
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) Trylock(ctx context.Context, key string,
	expiration time.Duration) (*Lock, error) {
	val := uuid.New().String()
	ok, err := c.client.SetNX(ctx, key, val, expiration).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		// 锁被别人抢占
		return nil, ErrFailedToPreemptLock
	}
	return &Lock{
		client: c.client,
		key: key,
		val: val,
		expiration: expiration,
		unlockCh: make(chan struct{}, 1),
	}, nil
}


type Lock struct {
	client redis.Cmdable
	key string
	val string
	expiration time.Duration
	unlockCh chan struct{}
}

func (l *Lock) Unlock(ctx context.Context) error {
	// 解锁存在两个步骤: 判断 redis 中的锁是不是自己的, 后删除
	// 需要同步 -> lua
	res, err := l.client.Eval(ctx, luaUnlock, []string{l.key}, l.val).Int64()
	defer func() {
		select {
		case l.unlockCh <- struct{}{}:
		default:
		}
	}()
	if err != nil {
		return err
	}
	if res != 1 {
		return ErrLockNotHold
	}
	return nil
}

func (l *Lock) Refresh(ctx context.Context) error {
	// 需使用 lua, 需同时进行两个操作: 检查当前锁是否是我的, 和刷新超时时间
	res, err := l.client.Eval(ctx, luaRefresh, []string{l.key}, l.val, l.expiration.Seconds()).Int64()
	if err != nil {
		return err
	}
	if res != 1 {
		return ErrLockNotHold
	}
	return nil
}

func (l *Lock) AutoRefresh(interval time.Duration, timeout time.Duration) error {
	timeoutCh := make(chan struct{}, 1)
	// 续约的时间间隔
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			// 刷新出现错误, 分情况处理
			err := l.Refresh(ctx)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				timeoutCh <- struct{}{}
			}
			if err != nil {
				return err
			}
		case <-timeoutCh:
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := l.Refresh(ctx)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				timeoutCh <- struct{}{}
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