package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

// shared vars for grpc reconnection backoff
var GrpcReconnectConfig = struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}{
	InitialDelay: 5 * time.Second,
	MaxDelay:     60 * time.Second,
	Multiplier:   2.0,
}

const cloudflare524Error = "524"

func MonitorGrpcConn(
	ctx context.Context, conn *grpc.ClientConn, onReconnect func(ctx context.Context) error,
) {
	firstReadySeen := false
	wasDisconnected := false

	for {
		select {
		case <-ctx.Done():
			fmt.Println("MonitorGrpcConn: exiting (context cancelled)")
			return
		default:
			currentState := conn.GetState()

			if conn.WaitForStateChange(ctx, currentState) {
				newState := conn.GetState()
				fmt.Println("MonitorGrpcConn: state changed", currentState.String(), "->", newState.String())

				// Track if we've seen the initial Ready state
				if newState == connectivity.Ready && !firstReadySeen {
					firstReadySeen = true
					wasDisconnected = false
					fmt.Println("MonitorGrpcConn: gRPC connection ready (initial)")
					continue
				}

				// Mark as disconnected when we hit a failure state
				if !wasDisconnected && newState == connectivity.TransientFailure ||
					newState == connectivity.Shutdown {
					wasDisconnected = true
					fmt.Println("MonitorGrpcConn: disconnected, state=", newState.String(), ", triggering reconnect")
					if err := onReconnect(ctx); err != nil {
						fmt.Println("MonitorGrpcConn: failed to reconnect:", err)
					} else {
						fmt.Println("MonitorGrpcConn: reconnect callback completed successfully")
					}
				}

				// Only trigger callback if we're recovering from a disconnection
				if newState == connectivity.Ready && wasDisconnected {
					wasDisconnected = false
					fmt.Println("MonitorGrpcConn: connection recovered (Ready after disconnect)")
				}
			}
		}
	}
}

// ShouldReconnect checks if a gRPC error should trigger a reconnection attempt
// and returns the backoff duration if reconnection should be attempted
func ShouldReconnect(err error) (bool, time.Duration) {
	if strings.Contains(err.Error(), cloudflare524Error) {
		// cloudflare 524 error is a timeout error, so we should reconnect
		return true, 5 * time.Second
	}

	st, ok := status.FromError(err)
	if !ok {
		return true, time.Second
	}

	switch st.Code() {
	case codes.ResourceExhausted:
		return true, 5 * time.Second // rate limited
	case codes.Canceled, codes.InvalidArgument: // bad request
		return false, 0
	case codes.Unavailable, codes.Internal, codes.DeadlineExceeded:
		return true, time.Second
	default:
		return true, time.Second
	}
}
