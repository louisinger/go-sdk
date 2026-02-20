package grpcclient

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestShouldReconnect(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		expectedRetry     bool
		expectedMinDelay  time.Duration
		expectedMaxDelay  time.Duration
	}{
		{
			name:             "codes.Unavailable should reconnect",
			err:              status.Error(codes.Unavailable, "server unavailable"),
			expectedRetry:    true,
			expectedMinDelay: time.Second,
			expectedMaxDelay: time.Second,
		},
		{
			name:             "codes.ResourceExhausted should reconnect with delay",
			err:              status.Error(codes.ResourceExhausted, "rate limited"),
			expectedRetry:    true,
			expectedMinDelay: 5 * time.Second,
			expectedMaxDelay: 5 * time.Second,
		},
		{
			name:             "codes.DeadlineExceeded should reconnect",
			err:              status.Error(codes.DeadlineExceeded, "timeout"),
			expectedRetry:    true,
			expectedMinDelay: time.Second,
			expectedMaxDelay: time.Second,
		},
		{
			name:             "codes.Internal should reconnect",
			err:              status.Error(codes.Internal, "internal error"),
			expectedRetry:    true,
			expectedMinDelay: time.Second,
			expectedMaxDelay: time.Second,
		},
		{
			name:             "codes.Canceled should NOT reconnect",
			err:              status.Error(codes.Canceled, "client cancelled"),
			expectedRetry:    false,
			expectedMinDelay: 0,
			expectedMaxDelay: 0,
		},
		{
			name:             "codes.InvalidArgument should NOT reconnect",
			err:              status.Error(codes.InvalidArgument, "bad request"),
			expectedRetry:    false,
			expectedMinDelay: 0,
			expectedMaxDelay: 0,
		},
		{
			name:             "unknown error should reconnect",
			err:              status.Error(codes.Unknown, "unknown error"),
			expectedRetry:    true,
			expectedMinDelay: time.Second,
			expectedMaxDelay: time.Second,
		},
		{
			name:             "non-gRPC error should reconnect",
			err:              &testError{msg: "some random error"},
			expectedRetry:    true,
			expectedMinDelay: time.Second,
			expectedMaxDelay: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, delay := shouldReconnect(tt.err)

			if retry != tt.expectedRetry {
				t.Errorf("shouldReconnect() retry = %v, want %v", retry, tt.expectedRetry)
			}

			if delay < tt.expectedMinDelay || delay > tt.expectedMaxDelay {
				t.Errorf("shouldReconnect() delay = %v, want between %v and %v",
					delay, tt.expectedMinDelay, tt.expectedMaxDelay)
			}
		})
	}
}

// testError is a custom error type for testing non-gRPC errors
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestBackoffConstants(t *testing.T) {
	if initialBackoff != time.Second {
		t.Errorf("initialBackoff = %v, want %v", initialBackoff, time.Second)
	}

	if maxBackoff != 30*time.Second {
		t.Errorf("maxBackoff = %v, want %v", maxBackoff, 30*time.Second)
	}
}
