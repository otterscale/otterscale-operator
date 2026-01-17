package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

var _ = Describe("IstioPoller", func() {
	const (
		shortDuration = 50 * time.Millisecond
		testTimeout   = 1 * time.Second
	)

	var (
		ctx    context.Context
		cancel context.CancelFunc
		poller *IstioPoller
	)

	// --- Helpers ---

	setupPoller := func() *IstioPoller {
		return &IstioPoller{
			Config: &rest.Config{},
		}
	}

	runPollerAsync := func(ctx context.Context, p *IstioPoller) <-chan error {
		errCh := make(chan error)
		go func() {
			defer close(errCh)
			errCh <- p.Start(ctx)
		}()
		return errCh
	}

	// --- Lifecycle ---

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		poller = setupPoller()
	})

	AfterEach(func() {
		cancel()
	})

	// --- Tests ---

	Context("Lifecycle Management", func() {
		It("should exit immediately if context is already cancelled", func() {
			cancel()
			err := poller.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should run continuously and stop gracefully upon cancellation", func() {
			done := runPollerAsync(ctx, poller)
			Consistently(done, shortDuration).Should(
				Not(Receive()),
				"Poller should be running and not return early",
			)
			cancel()
			Eventually(done, testTimeout).Should(
				Receive(BeNil()),
				"Poller should stop without error after context cancellation",
			)
		})
	})

	Context("Operational Behavior", func() {
		It("should continue polling when Istio is not detected", func() {
			done := runPollerAsync(ctx, poller)
			Consistently(done, shortDuration).ShouldNot(Receive())
			cancel()
			Eventually(done, testTimeout).Should(Receive(Succeed()))
		})
	})
})
