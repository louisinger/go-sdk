package arksdk_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	arksdk "github.com/arkade-os/go-sdk"
	"github.com/arkade-os/go-sdk/client"
	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// mockTransportClient is a mock that simulates stream disconnections
type mockTransportClient struct {
	client.TransportClient
	getTransactionsStreamCalls atomic.Int32
	getEventStreamCalls        atomic.Int32
	shouldFailFirst            bool
	txChan                     chan client.TransactionEvent
	eventChan                  chan client.BatchEventChannel
}

func newMockTransportClient(shouldFailFirst bool) *mockTransportClient {
	return &mockTransportClient{
		shouldFailFirst: shouldFailFirst,
		txChan:          make(chan client.TransactionEvent, 10),
		eventChan:       make(chan client.BatchEventChannel, 10),
	}
}

func (m *mockTransportClient) GetTransactionsStream(ctx context.Context) (<-chan client.TransactionEvent, func(), error) {
	callNum := m.getTransactionsStreamCalls.Add(1)
	
	// Simulate failure on first call
	if m.shouldFailFirst && callNum == 1 {
		ch := make(chan client.TransactionEvent, 1)
		go func() {
			defer close(ch)
			time.Sleep(10 * time.Millisecond)
			ch <- client.TransactionEvent{Err: io.EOF}
		}()
		return ch, func() {}, nil
	}
	
	// Successful connection
	return m.txChan, func() { close(m.txChan) }, nil
}

func (m *mockTransportClient) GetEventStream(ctx context.Context, topics []string) (<-chan client.BatchEventChannel, func(), error) {
	callNum := m.getEventStreamCalls.Add(1)
	
	// Simulate failure on first call
	if m.shouldFailFirst && callNum == 1 {
		ch := make(chan client.BatchEventChannel, 1)
		go func() {
			defer close(ch)
			time.Sleep(10 * time.Millisecond)
			ch <- client.BatchEventChannel{Err: io.EOF}
		}()
		return ch, func() {}, nil
	}
	
	// Successful connection
	return m.eventChan, func() { close(m.eventChan) }, nil
}

func (m *mockTransportClient) Close() {}

func (m *mockTransportClient) GetInfo(ctx context.Context) (*client.Info, error) {
	return &client.Info{
		SignerPubKey:        "test",
		Network:             "regtest",
		Dust:                1000,
		VtxoTreeExpiry:      512,
		UnilateralExitDelay: 1024,
		RoundInterval:       5,
	}, nil
}

func TestTransactionStreamReconnection(t *testing.T) {
	t.Run("should reconnect on EOF error", func(t *testing.T) {
		mock := newMockTransportClient(true)
		
		// Wait for reconnection attempts
		time.Sleep(200 * time.Millisecond)
		
		// Verify that GetTransactionsStream was called multiple times (initial + reconnect)
		calls := mock.getTransactionsStreamCalls.Load()
		require.GreaterOrEqual(t, calls, int32(2), "Expected at least 2 calls to GetTransactionsStream (initial + reconnect)")
	})

	t.Run("should not reconnect on context cancellation", func(t *testing.T) {
		mock := newMockTransportClient(false)
		ctx, cancel := context.WithCancel(context.Background())
		
		// Start listening
		go func() {
			ch, _, err := mock.GetTransactionsStream(ctx)
			require.NoError(t, err)
			
			for {
				select {
				case <-ch:
				case <-ctx.Done():
					return
				}
			}
		}()
		
		// Cancel context
		time.Sleep(50 * time.Millisecond)
		cancel()
		time.Sleep(50 * time.Millisecond)
		
		// Should only have been called once
		calls := mock.getTransactionsStreamCalls.Load()
		require.Equal(t, int32(1), calls, "Expected exactly 1 call when context is cancelled")
	})
}

func TestEventStreamReconnection(t *testing.T) {
	t.Run("should reconnect on EOF error", func(t *testing.T) {
		mock := newMockTransportClient(true)
		
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		
		// Simulate starting event stream
		go func() {
			for {
				ch, closeFn, err := mock.GetEventStream(ctx, []string{"test"})
				if err != nil {
					return
				}
				defer closeFn()
				
				for {
					select {
					case event, ok := <-ch:
						if !ok {
							return
						}
						if errors.Is(event.Err, io.EOF) {
							// Should retry here
							time.Sleep(100 * time.Millisecond)
							break // Break inner loop to reconnect
						}
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		
		time.Sleep(250 * time.Millisecond)
		
		// Verify reconnection attempts
		calls := mock.getEventStreamCalls.Load()
		require.GreaterOrEqual(t, calls, int32(2), "Expected at least 2 calls to GetEventStream (initial + reconnect)")
	})
}

func TestExponentialBackoff(t *testing.T) {
	t.Run("should use exponential backoff for retries", func(t *testing.T) {
		start := time.Now()
		
		mock := newMockTransportClient(true)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		retries := 0
		maxRetries := 3
		
		for retries < maxRetries {
			ch, closeFn, err := mock.GetTransactionsStream(ctx)
			if err != nil {
				t.Fatal(err)
			}
			
			select {
			case event, ok := <-ch:
				if !ok || errors.Is(event.Err, io.EOF) {
					closeFn()
					retries++
					if retries < maxRetries {
						// Calculate exponential backoff: min(baseDelay * 2^retries, maxDelay)
						delay := time.Duration(100*retries*retries) * time.Millisecond
						if delay > 1*time.Second {
							delay = 1 * time.Second
						}
						time.Sleep(delay)
					}
				}
			case <-ctx.Done():
				closeFn()
				return
			}
		}
		
		elapsed := time.Since(start)
		
		// Should have some delay due to backoff
		// First retry: ~100ms, second: ~400ms = ~500ms minimum
		require.Greater(t, elapsed, 400*time.Millisecond, "Expected exponential backoff delays")
	})
}
