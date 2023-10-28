package redis_lock

import (
	"context"
	_ "embed"
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

var (
	ErrFailedToPreemptLock = errors.New("redis-lock: 抢锁失败")
	ErrLockNotHold         = errors.New("redis-lock: 你没有持有锁")
	
	//go:embed lua/unlock.lua
	luaUnlock string
)

type Client struct {
	client redis.Cmdable
}

func NewClient(client redis.Cmdable) *Client {
	return &Client{
		client: client,
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
