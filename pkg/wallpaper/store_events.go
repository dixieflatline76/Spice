package wallpaper

import "context"

// notifyUpdateLocked signals that the store has been updated.
// CALLER MUST HOLD s.mu.Lock()
func (s *ImageStore) notifyUpdateLocked() {
	// Close the current channel to broadcast to all waiters
	select {
	case <-s.updateCh:
		// Already closed, do nothing (shouldn't happen if we strictly renew)
	default:
		close(s.updateCh)
		// Immediately create a fresh channel for future waiters
		s.updateCh = make(chan struct{})
	}
}

// WaitForImages blocks until the store has at least one image or the context is cancelled.
func (s *ImageStore) WaitForImages(ctx context.Context) error {
	for {
		s.mu.RLock()
		if len(s.images) > 0 {
			s.mu.RUnlock()
			return nil
		}
		// Grab the current channel while holding the lock
		ch := s.updateCh
		s.mu.RUnlock()

		// Wait for signal or context
		select {
		case <-ch:
			// Store updated, loop back to check count
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// GetUpdateChannel returns a channel that signals when the store content changes.
func (s *ImageStore) GetUpdateChannel() <-chan struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updateCh
}
