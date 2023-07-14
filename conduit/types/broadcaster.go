package types

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

type Broadcaster[T any] struct {
	subscribers map[*Subscriber[T]]struct{}
	mu          sync.RWMutex

	logger *log.Logger
}

type Subscriber[T any] struct {
	Chan   chan T
	closed chan struct{}
	once   sync.Once
}

func NewBroadcaster[T any](logger *log.Logger) *Broadcaster[T] {
	return &Broadcaster[T]{
		subscribers: make(map[*Subscriber[T]]struct{}),
		logger:      logger,
	}
}

func (b *Broadcaster[T]) Subscribe() *Subscriber[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscriber[T]{
		Chan:   make(chan T, 1),
		closed: make(chan struct{}),
	}

	b.subscribers[sub] = struct{}{}

	b.logger.Debugf("Subscribe() resulting in %d total subscribers", len(b.subscribers))
	return sub
}

func (b *Broadcaster[T]) Broadcast(t T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		go func(sub *Subscriber[T], t T) {
			select {
			case <-sub.closed:
			case sub.Chan <- t:
			}
		}(sub, t)
	}
}

func (s *Subscriber[T]) Get() T {
	return <-s.Chan
}

func (s *Subscriber[T]) Close() {
	s.once.Do(func() {
		close(s.Chan)
		close(s.closed)
	})
}

func (b *Broadcaster[T]) Unsubscribe(s *Subscriber[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.subscribers, s)
	s.Close()
}
