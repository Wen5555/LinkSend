package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/app"
)

func TestLocalDirectDemo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := app.RunLocalDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Relay || !report.AuthenticatedTLS || report.SourceDigest != report.ReceivedDigest {
		t.Fatalf("unexpected demo report: %+v", report)
	}
	if report.SignalingForwardedBytes >= uint64(report.FileBytes) {
		t.Fatalf("signaling carried too many bytes: %+v", report)
	}
}
