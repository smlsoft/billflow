package catalog

import (
	"errors"
	"testing"

	"go.uber.org/zap"
)

func TestStartEmbedCodesRejectsConcurrentRun(t *testing.T) {
	svc := NewSMLCatalogService(nil, "", nil, zap.NewNop())
	svc.embedRunning.Store(1)
	defer svc.embedRunning.Store(0)

	started, err := svc.StartEmbedCodes(NewEmbeddingService("key"), []string{"SKU-1"}, nil)
	if started {
		t.Fatal("started = true, want false")
	}
	if !errors.Is(err, ErrEmbedAlreadyRunning) {
		t.Fatalf("err = %v, want ErrEmbedAlreadyRunning", err)
	}
}

func TestStartEmbedAllPendingReservesTheWorkerBeforeStarting(t *testing.T) {
	svc := NewSMLCatalogService(nil, "", nil, zap.NewNop())
	svc.embedRunning.Store(1)
	defer svc.embedRunning.Store(0)

	started, err := svc.StartEmbedAllPending(NewEmbeddingService("key"), nil)
	if started {
		t.Fatal("started = true, want false")
	}
	if !errors.Is(err, ErrEmbedAlreadyRunning) {
		t.Fatalf("err = %v, want ErrEmbedAlreadyRunning", err)
	}
}
