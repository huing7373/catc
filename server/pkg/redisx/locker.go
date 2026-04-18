package redisx

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var unlockScript = redis.NewScript(
	`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`,
)

type Locker struct {
	cmd        redis.Cmdable
	instanceID string
}

func NewLocker(cmd redis.Cmdable) *Locker {
	return &Locker{
		cmd:        cmd,
		instanceID: uuid.New().String(),
	}
}

func (l *Locker) InstanceID() string { return l.instanceID }

func (l *Locker) WithLock(ctx context.Context, name string, ttl time.Duration, fn func() error) error {
	key := "lock:cron:" + name

	ok, err := l.cmd.SetNX(ctx, key, l.instanceID, ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	defer unlockScript.Run(ctx, l.cmd, []string{key}, l.instanceID)

	return fn()
}
