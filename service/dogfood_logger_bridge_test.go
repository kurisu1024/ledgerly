package service

import (
	"fmt"
	"log/slog"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestDogfoodLoggerBridgesToZapCore is the RED case for the slog-over-zap
// bridge (zapslog.NewHandler(base.Core())): every record written through
// df.logger must land in the same zap core service/ already logs to,
// whether dogfood is on or off. Off, the bridge is the whole story (no SDK
// teeing); on, ledgerly.Handler tees unconditionally to its next handler
// (ADR-0001) — the SDK's own capture/delivery pipeline never suppresses the
// app's own log output, so this must hold identically both ways.
func TestDogfoodLoggerBridgesToZapCore(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		enabled := enabled
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			core, logs := observer.New(zapcore.InfoLevel)
			base := zap.New(core)

			df, err := newDogfood(base, dogfoodConfig{
				enabled:   enabled,
				bufferDir: t.TempDir(),
				eventsURL: "http://127.0.0.1:0/v1/events",
				rulesURL:  "http://127.0.0.1:0/v1/rules",
			})
			if err != nil {
				t.Fatalf("newDogfood: %v", err)
			}
			if df.logger == nil {
				t.Fatalf("df.logger is nil")
			}

			df.logger.Info("service started",
				slog.String("event-type", "service.started"),
				slog.Group("metadata", slog.String("backend", "memory")),
			)

			if got := logs.Len(); got != 1 {
				t.Fatalf("zap core observed %d entries, want exactly 1 (the bridged record)", got)
			}
			entry := logs.All()[0]
			if entry.Message != "service started" {
				t.Fatalf("bridged entry message = %q, want %q", entry.Message, "service started")
			}
		})
	}
}
