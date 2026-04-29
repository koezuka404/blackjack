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
	ensureLegacyUsersEmailFn                   = ensureLegacyUsersEmail
	execSQLFn                                  = execSQLGorm
)

func Migrate(gdb *gorm.DB) error {
	if err := ensureLegacyUsersEmailFn(gdb); err != nil {
		return err
	}
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

// ensureLegacyUsersEmail は、email 列追加前の users 行に NULL が残っていると
// AutoMigrate の「NOT NULL 列追加」が失敗するため、先に nullable 列で追加・埋め戻しする。
func ensureLegacyUsersEmail(gdb *gorm.DB) error {
	if gdb == nil || gdb.Dialector == nil {
		return nil
	}
	switch gdb.Dialector.Name() {
	case "postgres":
		return ensureLegacyUsersEmailPostgres(gdb)
	case "sqlite":
		return ensureLegacyUsersEmailSQLite(gdb)
	default:
		return nil
	}
}

func ensureLegacyUsersEmailPostgres(gdb *gorm.DB) error {
	// users が無い DB は AutoMigrate が作るので何もしない
	var n int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'users'`).Scan(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	// email 列が無ければ NULL 許容で追加（既存行は NULL のまま）
	if err := execSQLFn(gdb, `
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'email'
  ) THEN
    ALTER TABLE users ADD COLUMN email VARCHAR(255);
  END IF;
END $$;`); err != nil {
		return err
	}

	// 空・NULL の email を一意のダミーに（ログインはメールなので要パスワードリセット / アカウント再登録想定）
	if err := execSQLFn(gdb, `
UPDATE users
SET email = 'legacy-' || REPLACE(id::text, '-', '') || '@migrated.invalid'
WHERE email IS NULL OR BTRIM(COALESCE(email, '')) = '';`); err != nil {
		return err
	}

	// まだ NULL があれば失敗させる（通常は起きない）
	if err := execSQLFn(gdb, `ALTER TABLE users ALTER COLUMN email SET NOT NULL`); err != nil {
		return err
	}
	return nil
}

func ensureLegacyUsersEmailSQLite(gdb *gorm.DB) error {
	var n int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return nil
	}

	var hasEmail int64
	if err := gdb.Raw(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='email'`).Scan(&hasEmail).Error; err != nil {
		return err
	}
	if hasEmail == 0 {
		if err := execSQLFn(gdb, `ALTER TABLE users ADD COLUMN email VARCHAR(255)`); err != nil {
			return err
		}
	}

	return execSQLFn(gdb, `
UPDATE users
SET email = 'legacy-' || REPLACE(id, '-', '') || '@migrated.invalid'
WHERE email IS NULL OR TRIM(COALESCE(email, '')) = '';`)
}

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
