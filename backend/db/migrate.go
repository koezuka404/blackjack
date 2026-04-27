package db

import (
	"fmt"

	"blackjack/backend/repository"

	"gorm.io/gorm"
)

func autoMigrateModels(gdb *gorm.DB, models ...any) error {
	return gdb.AutoMigrate(models...)
}

func execSQLGorm(gdb *gorm.DB, q string) error {
	return gdb.Exec(q).Error
}

var (
	autoMigrateFn                              = autoMigrateModels
	ensureForeignKeysFn                        = ensureForeignKeys
	ensurePlayerStatesSessionSeatUniqueIndexFn = ensurePlayerStatesSessionSeatUniqueIndex
	execSQLFn                                  = execSQLGorm
)

func Migrate(gdb *gorm.DB) error {
	if err := autoMigrateFn(gdb,
		&repository.RoomRecord{},
		&repository.RoomPlayerRecord{},
		&repository.GameSessionRecord{},
		&repository.PlayerStateRecord{},
		&repository.DealerStateRecord{},
		&repository.ActionLogRecord{},
		&repository.RematchVoteRecord{},
		&repository.RoundLogRecord{},
		&repository.UserRecord{},
		&repository.SessionRecord{},
	); err != nil {
		return err
	}
	if err := ensureForeignKeysFn(gdb); err != nil {
		return err
	}
	return ensurePlayerStatesSessionSeatUniqueIndexFn(gdb)
}

// ensurePlayerStatesSessionSeatUniqueIndex replaces a mistaken GORM definition where
// uniqueIndex:ux_player_session_seat lived only on seat_no, which enforced uniqueness
// of seat numbers across all sessions. The correct invariant is unique (session_id, seat_no).
func ensurePlayerStatesSessionSeatUniqueIndex(gdb *gorm.DB) error {
	_ = execSQLFn(gdb, `ALTER TABLE player_states DROP CONSTRAINT IF EXISTS ux_player_session_seat`)
	_ = execSQLFn(gdb, `DROP INDEX IF EXISTS ux_player_session_seat`)
	return execSQLFn(gdb, `CREATE UNIQUE INDEX IF NOT EXISTS ux_player_session_seat ON player_states (session_id, seat_no)`)
}

func ensureForeignKeys(gdb *gorm.DB) error {
	stmts := []struct {
		table      string
		name       string
		definition string
	}{
		{"game_sessions", "fk_game_sessions_room", "FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE"},
		{"rooms", "fk_rooms_current_session", "FOREIGN KEY (current_session_id) REFERENCES game_sessions(id) ON DELETE SET NULL"},
		{"room_players", "fk_room_players_room", "FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE"},
		{"player_states", "fk_player_states_session", "FOREIGN KEY (session_id) REFERENCES game_sessions(id) ON DELETE CASCADE"},
		{"dealer_states", "fk_dealer_states_session", "FOREIGN KEY (session_id) REFERENCES game_sessions(id) ON DELETE CASCADE"},
		{"action_logs", "fk_action_logs_session", "FOREIGN KEY (session_id) REFERENCES game_sessions(id) ON DELETE CASCADE"},
		{"rematch_votes", "fk_rematch_votes_session", "FOREIGN KEY (session_id) REFERENCES game_sessions(id) ON DELETE CASCADE"},
		{"round_logs", "fk_round_logs_session", "FOREIGN KEY (session_id) REFERENCES game_sessions(id) ON DELETE CASCADE"},
		{"sessions", "fk_sessions_user", "FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE"},
	}
	for _, s := range stmts {
		q := fmt.Sprintf(
			"DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s') THEN ALTER TABLE %s ADD CONSTRAINT %s %s; END IF; END $$;",
			s.name, s.table, s.name, s.definition,
		)
		if err := execSQLFn(gdb, q); err != nil {
			return err
		}
	}
	return nil
}
