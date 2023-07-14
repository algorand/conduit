package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBroadcaster(t *testing.T) {
	b := NewBroadcaster[int](nil)

	// Test Subscribe
	sub1 := b.Subscribe()
	require.NotNil(t, sub1, "Expected subscriber, got nil")
	sub2 := b.Subscribe()
	require.NotNil(t, sub2, "Expected subscriber, got nil")

	// Test Broadcast and Get
	b.Broadcast(10)
	val := sub1.Get()
	require.Equal(t, 10, val, "Expected 10, got %d", val)
	val = sub2.Get()
	require.Equal(t, 10, val, "Expected 10, got %d", val)

	// Test Close
	sub1.Close()
	if sub1 != nil {
		select {
		case <-sub1.closed:
		case <-time.After(time.Second):
			t.Error("Expected subscriber to be closed")
		}
	}

	// Test Unsubscribe
	b.Unsubscribe(sub1)
	b.mu.RLock()
	_, ok := b.subscribers[sub1]
	b.mu.RUnlock()
	require.False(t, ok, "Expected subscriber to be unsubscribed")

	// Test that closed subscriber does not receive broadcasts
	b.Broadcast(20)
	val = sub2.Get()
	require.Equal(t, 20, val, "Expected 20, got %d", val)

	// Check if sub1.ch is closed
	_, ok = <-sub1.ch
	require.False(t, ok, "Expected sub1.ch to be closed")
}
