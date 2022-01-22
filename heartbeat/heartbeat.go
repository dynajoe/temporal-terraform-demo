package heartbeat

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
)

func Begin(ctx context.Context, frequency time.Duration) (context.Context, func()) {
	// Create a context that can be canceled as soon as the worker is stopped
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-activity.GetWorkerStopChannel(ctx):
		case <-ctx.Done():
		}
		cancel()
	}()

	go startHeartbeats(ctx, frequency)

	return ctx, cancel
}

func startHeartbeats(ctx context.Context, frequency time.Duration) {
	ticker := time.NewTicker(frequency)
	defer ticker.Stop()

	activity.RecordHeartbeat(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			activity.RecordHeartbeat(ctx)
		}
	}
}
