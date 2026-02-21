package reconciler

import (
	"context"
	"time"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/config"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/repository"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/store"
)

// Reconciler periodically syncs hot-key Redis counts with the database.
type Reconciler struct {
	store  store.FollowStore
	repo   repository.FollowRepository
	cfg    config.ReconcilerConfig
	quit   chan struct{}
	doneCh chan struct{}
}

// New creates a new Reconciler.
func New(store store.FollowStore, repo repository.FollowRepository, cfg config.ReconcilerConfig) *Reconciler {
	return &Reconciler{
		store:  store,
		repo:   repo,
		cfg:    cfg,
		quit:   make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start launches the reconciler in a background goroutine.
func (r *Reconciler) Start(ctx context.Context) {
	go r.run(ctx)
}

// Stop signals the reconciler to stop and returns immediately.
// Call Done() to wait for it to exit.
func (r *Reconciler) Stop() {
	close(r.quit)
}

// Done returns a channel that is closed when the reconciler has fully stopped.
func (r *Reconciler) Done() <-chan struct{} {
	return r.doneCh
}

func (r *Reconciler) run(ctx context.Context) {
	defer close(r.doneCh)

	interval := r.cfg.Interval
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.quit:
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	l := pkglog.L()
	l.Info().Msg("reconciler: starting hot-key reconciliation")

	topN := int64(r.cfg.TopN)
	if topN <= 0 {
		topN = 100
	}

	// 1. Fetch top-N hot keys
	userIDs, err := r.store.GetTopHotKeys(ctx, topN)
	if err != nil {
		l.Error().Err(err).Msg("reconciler: failed to get top hot keys")
		return
	}

	if len(userIDs) == 0 {
		l.Info().Msg("reconciler: no hot keys to reconcile")
		return
	}

	// 2. For each hot key, fetch count from DB and update Redis
	for _, userID := range userIDs {
		count, err := r.repo.GetFollowersCount(ctx, userID)
		if err != nil {
			l.Error().Err(err).Str("user_id", userID).Msg("reconciler: failed to get followers count from db")
			continue
		}
		if err := r.store.SetFollowersCount(ctx, userID, count); err != nil {
			l.Error().Err(err).Str("user_id", userID).Msg("reconciler: failed to set followers count in redis")
		}
	}

	// 3. Reset hot key scores for the next cycle
	if err := r.store.ResetHotKeyScores(ctx); err != nil {
		l.Error().Err(err).Msg("reconciler: failed to reset hot key scores")
	}

	l.Info().Int("count", len(userIDs)).Msg("reconciler: hot-key reconciliation complete")
}
