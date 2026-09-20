package store

import (
	"fmt"
)

func (s *Store) PruneOldEventsAndExitedAgents(retentionCutoffMs int64, exitedRetentionCutoffMs int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Delete events older than retention cutoff (e.g. 48 hours)
	_, err = tx.Exec("DELETE FROM telemetry_events WHERE timestamp < ?", retentionCutoffMs)
	if err != nil {
		return fmt.Errorf("deleting old telemetry events: %w", err)
	}

	// 2. Delete EXITED agents untouched for >30 days
	_, err = tx.Exec(`
		DELETE FROM agents
		WHERE activity = 'IDLE' AND current_run_id IS NULL AND updated_at < ?
	`, exitedRetentionCutoffMs)
	if err != nil {
		return fmt.Errorf("deleting old exited agents: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// 3. Execute incremental vacuum
	_, _ = s.db.Exec("PRAGMA incremental_vacuum(1000);")
	return nil
}
