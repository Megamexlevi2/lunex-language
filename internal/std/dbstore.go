package std

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var validFieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const dbDirName = ".lunex/data"

type sqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

type sqliteStore struct {
	path string
	sql  *sql.DB
	mu   sync.Mutex
	opMu sync.RWMutex
}

var storeRegistry = struct {
	mu     sync.Mutex
	stores map[string]*sqliteStore
}{stores: make(map[string]*sqliteStore)}

func resolveDBPath(name string) (string, error) {
	if name == "" {
		name = "default"
	}
	if filepath.Ext(name) != ".db" {
		name += ".db"
	}
	if filepath.IsAbs(name) {
		return filepath.Clean(name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cwd, dbDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(name)), nil
}

func openStore(name string) (*sqliteStore, error) {
	path, err := resolveDBPath(name)
	if err != nil {
		return nil, err
	}
	storeRegistry.mu.Lock()
	defer storeRegistry.mu.Unlock()
	if s, ok := storeRegistry.stores[path]; ok {
		return s, nil
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: failed to open %s: %w", path, err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS __lunex_meta__ (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		database.Close()
		return nil, fmt.Errorf("db: failed to initialize %s: %w", path, err)
	}
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS __lunex_seqs__ (name TEXT PRIMARY KEY, value INTEGER NOT NULL DEFAULT 0)`); err != nil {
		database.Close()
		return nil, fmt.Errorf("db: failed to initialize %s: %w", path, err)
	}
	s := &sqliteStore{path: path, sql: database}
	storeRegistry.stores[path] = s
	return s, nil
}

func dropStoreFile(name string) error {
	path, err := resolveDBPath(name)
	if err != nil {
		return err
	}
	storeRegistry.mu.Lock()
	s := storeRegistry.stores[path]
	if s != nil {
		s.opMu.Lock()
		err = s.sql.Close()
		delete(storeRegistry.stores, path)
		s.opMu.Unlock()
		if err != nil {
			storeRegistry.mu.Unlock()
			return err
		}
	}
	storeRegistry.mu.Unlock()
	var firstErr error
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if removeErr := os.Remove(path + suffix); removeErr != nil && !os.IsNotExist(removeErr) && firstErr == nil {
			firstErr = removeErr
		}
	}
	return firstErr
}

func listStoreFiles() []string {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	dir := filepath.Join(cwd, dbDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}
		}
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".db" {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".db"))
	}
	sort.Strings(out)
	return out
}

func sqlIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s *sqliteStore) withRead(fn func(sqlExecutor) error) error {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(s.sql)
}

func (s *sqliteStore) withWrite(fn func(sqlExecutor) error) error {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(s.sql)
}

func (s *sqliteStore) ensureTable(name string) error {
	return s.ensureTableTx(nil, name)
}

func (s *sqliteStore) ensureTableTx(tx *sql.Tx, name string) error {
	if !validFieldName.MatchString(name) {
		return fmt.Errorf("db: invalid table name %q", name)
	}
	exec := sqlExecutor(s.sql)
	if tx != nil {
		exec = tx
	}
	if tx == nil {
		s.opMu.RLock()
		defer s.opMu.RUnlock()
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	_, err := exec.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (id TEXT PRIMARY KEY, doc TEXT NOT NULL)`, sqlIdent(name)))
	return err
}

func (s *sqliteStore) dropTable(name string) error {
	return s.dropTableTx(nil, name)
}

func (s *sqliteStore) dropTableTx(tx *sql.Tx, name string) error {
	if !validFieldName.MatchString(name) {
		return fmt.Errorf("db: invalid table name %q", name)
	}
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		if _, err := exec.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, sqlIdent(name))); err != nil {
			return err
		}
		for _, key := range []string{"schema:" + name, "indexes:" + name} {
			if _, err := exec.Exec(`DELETE FROM __lunex_meta__ WHERE key = ?`, key); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sqliteStore) clearTable(name string) error {
	return s.clearTableTx(nil, name)
}

func (s *sqliteStore) clearTableTx(tx *sql.Tx, name string) error {
	if !validFieldName.MatchString(name) {
		return fmt.Errorf("db: invalid table name %q", name)
	}
	exec := sqlExecutor(s.sql)
	if tx != nil {
		exec = tx
	} else {
		s.opMu.RLock()
		defer s.opMu.RUnlock()
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	_, err := exec.Exec(fmt.Sprintf(`DELETE FROM %s`, sqlIdent(name)))
	return err
}

func (s *sqliteStore) listTables() ([]string, error) {
	return s.listTablesTx(nil)
}

func (s *sqliteStore) listTablesTx(tx *sql.Tx) ([]string, error) {
	var names []string
	run := func(exec sqlExecutor) error {
		rows, err := exec.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE '__lunex_%' ORDER BY name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			names = append(names, name)
		}
		return rows.Err()
	}
	if tx != nil {
		return names, run(tx)
	}
	return names, s.withRead(run)
}

func (s *sqliteStore) insertRow(table, id string, docJSON []byte) error {
	return s.insertRowTx(nil, table, id, docJSON)
}

func (s *sqliteStore) insertRowTx(tx *sql.Tx, table, id string, docJSON []byte) error {
	if !validFieldName.MatchString(table) {
		return fmt.Errorf("db: invalid table name %q", table)
	}
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		_, err := exec.Exec(fmt.Sprintf(`INSERT INTO %s (id, doc) VALUES (?, ?)`, sqlIdent(table)), id, string(docJSON))
		return err
	})
}

func (s *sqliteStore) updateRow(table, id string, docJSON []byte) error {
	return s.updateRowTx(nil, table, id, docJSON)
}

func (s *sqliteStore) updateRowTx(tx *sql.Tx, table, id string, docJSON []byte) error {
	if !validFieldName.MatchString(table) {
		return fmt.Errorf("db: invalid table name %q", table)
	}
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		result, err := exec.Exec(fmt.Sprintf(`UPDATE %s SET doc = ? WHERE id = ?`, sqlIdent(table)), string(docJSON), id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("db: row %q not found in table %q", id, table)
		}
		return nil
	})
}

func (s *sqliteStore) deleteRow(table, id string) error {
	return s.deleteRowTx(nil, table, id)
}

func (s *sqliteStore) deleteRowTx(tx *sql.Tx, table, id string) error {
	if !validFieldName.MatchString(table) {
		return fmt.Errorf("db: invalid table name %q", table)
	}
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		_, err := exec.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, sqlIdent(table)), id)
		return err
	})
}

func (s *sqliteStore) clearRows(table string) error {
	return s.clearTable(table)
}

func (s *sqliteStore) clearRowsTx(tx *sql.Tx, table string) error {
	return s.clearTableTx(tx, table)
}

type rawRow struct {
	ID  string
	Doc map[string]interface{}
}

func (s *sqliteStore) loadAll(table string) ([]rawRow, error) {
	return s.loadAllTx(nil, table)
}

func (s *sqliteStore) loadAllTx(tx *sql.Tx, table string) ([]rawRow, error) {
	if !validFieldName.MatchString(table) {
		return nil, fmt.Errorf("db: invalid table name %q", table)
	}
	var out []rawRow
	run := func(exec sqlExecutor) error {
		rows, err := exec.Query(fmt.Sprintf(`SELECT id, doc FROM %s ORDER BY rowid ASC`, sqlIdent(table)))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, docStr string
			if err := rows.Scan(&id, &docStr); err != nil {
				return err
			}
			var doc map[string]interface{}
			if err := json.Unmarshal([]byte(docStr), &doc); err != nil {
				return fmt.Errorf("db: corrupt row %s in table %s: %w", id, table, err)
			}
			out = append(out, rawRow{ID: id, Doc: doc})
		}
		return rows.Err()
	}
	if tx != nil {
		if err := run(tx); err != nil {
			return nil, err
		}
		return out, nil
	}
	if err := s.withRead(run); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *sqliteStore) dropIndexTx(tx *sql.Tx, table, indexName string) error {
	if !validFieldName.MatchString(table) || !validFieldName.MatchString(indexName) {
		return fmt.Errorf("db: invalid table or index name")
	}
	name := "idx_" + table + "_" + indexName
	stmt := fmt.Sprintf(`DROP INDEX IF EXISTS %s`, sqlIdent(name))
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		_, err := exec.Exec(stmt)
		return err
	})
}

func (s *sqliteStore) createIndex(table, indexName string, fields []string, unique bool) error {
	return s.createIndexTx(nil, table, indexName, fields, unique)
}

func (s *sqliteStore) createIndexTx(tx *sql.Tx, table, indexName string, fields []string, unique bool) error {
	if !validFieldName.MatchString(table) || !validFieldName.MatchString(indexName) {
		return fmt.Errorf("db: invalid table or index name")
	}
	if len(fields) == 0 {
		return fmt.Errorf("db: index requires at least one field")
	}
	exprs := make([]string, len(fields))
	for i, f := range fields {
		if f == "_id" || f == "id" {
			exprs[i] = "id"
			continue
		}
		if !validFieldName.MatchString(f) {
			return fmt.Errorf("db: invalid field name %q", f)
		}
		exprs[i] = fmt.Sprintf(`json_extract(doc, '$.%s')`, strings.ReplaceAll(f, `'`, `''`))
	}
	uniqueKw := ""
	if unique {
		uniqueKw = "UNIQUE "
	}
	name := "idx_" + table + "_" + indexName
	dropStmt := fmt.Sprintf(`DROP INDEX IF EXISTS %s`, sqlIdent(name))
	stmt := fmt.Sprintf(`CREATE %sINDEX %s ON %s (%s)`, uniqueKw, sqlIdent(name), sqlIdent(table), strings.Join(exprs, ", "))
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		if _, err := exec.Exec(dropStmt); err != nil {
			return err
		}
		_, err := exec.Exec(stmt)
		return err
	})
}

func (s *sqliteStore) nextSeq(name string) (int64, error) {
	return s.nextSeqTx(nil, name)
}

func (s *sqliteStore) nextSeqTx(tx *sql.Tx, name string) (int64, error) {
	run := func(exec sqlExecutor) (int64, error) {
		if tx != nil {
			if _, err := exec.Exec(`INSERT INTO __lunex_seqs__ (name, value) VALUES (?, 1) ON CONFLICT(name) DO UPDATE SET value = value + 1`, name); err != nil {
				return 0, err
			}
			var value int64
			if err := exec.QueryRow(`SELECT value FROM __lunex_seqs__ WHERE name = ?`, name).Scan(&value); err != nil {
				return 0, err
			}
			return value, nil
		}
		transaction, err := s.sql.Begin()
		if err != nil {
			return 0, err
		}
		if _, err := transaction.Exec(`INSERT INTO __lunex_seqs__ (name, value) VALUES (?, 1) ON CONFLICT(name) DO UPDATE SET value = value + 1`, name); err != nil {
			transaction.Rollback()
			return 0, err
		}
		var value int64
		if err := transaction.QueryRow(`SELECT value FROM __lunex_seqs__ WHERE name = ?`, name).Scan(&value); err != nil {
			transaction.Rollback()
			return 0, err
		}
		if err := transaction.Commit(); err != nil {
			return 0, err
		}
		return value, nil
	}
	if tx != nil {
		return run(tx)
	}
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return run(s.sql)
}

func (s *sqliteStore) getMeta(key string) (string, bool, error) {
	var value string
	err := s.withRead(func(exec sqlExecutor) error {
		return exec.QueryRow(`SELECT value FROM __lunex_meta__ WHERE key = ?`, key).Scan(&value)
	})
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *sqliteStore) getMetaTx(tx *sql.Tx, key string) (string, bool, error) {
	if tx == nil {
		return s.getMeta(key)
	}
	var value string
	err := tx.QueryRow(`SELECT value FROM __lunex_meta__ WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *sqliteStore) setMeta(key, value string) error {
	return s.setMetaTx(nil, key, value)
}

func (s *sqliteStore) setMetaTx(tx *sql.Tx, key, value string) error {
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		_, err := exec.Exec(`INSERT INTO __lunex_meta__ (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
		return err
	})
}

func (s *sqliteStore) deleteMeta(keys ...string) error {
	return s.deleteMetaTx(nil, keys...)
}

func (s *sqliteStore) deleteMetaTx(tx *sql.Tx, keys ...string) error {
	return s.execTxOrWrite(tx, func(exec sqlExecutor) error {
		for _, key := range keys {
			if _, err := exec.Exec(`DELETE FROM __lunex_meta__ WHERE key = ?`, key); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sqliteStore) begin() (*sql.Tx, error) {
	s.opMu.Lock()
	tx, err := s.sql.Begin()
	if err != nil {
		s.opMu.Unlock()
		return nil, err
	}
	return tx, nil
}

func (s *sqliteStore) finishTransaction() {
	s.opMu.Unlock()
}

func (s *sqliteStore) close() error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	err := s.sql.Close()
	s.mu.Unlock()
	storeRegistry.mu.Lock()
	if current, ok := storeRegistry.stores[s.path]; ok && current == s {
		delete(storeRegistry.stores, s.path)
	}
	storeRegistry.mu.Unlock()
	return err
}

func (s *sqliteStore) execTxOrWrite(tx *sql.Tx, fn func(sqlExecutor) error) error {
	if tx != nil {
		return fn(tx)
	}
	return s.withWrite(fn)
}
