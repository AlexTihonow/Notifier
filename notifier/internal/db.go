package notifier

import (
	"database/sql"
	"log"
	"github.com/go-sql-driver/mysql"
	"time"
	"strings"
	"my-mailer/internal/configs"
)

type Mailing struct {
	ID         string
	Body       string
	Recipients string
	Subject    string
	Target     string
}

func OpenDB(conf configs.Config) (*sql.DB, error){
	cfg := mysql.Config{
		User:   conf.DbUser,
		Passwd: conf.DbPass,
		Addr:   conf.DbAddress,
		DBName: conf.DbName,
		Net:    "tcp",
		AllowNativePasswords: true,
	}

	connector, err := mysql.NewConnector(&cfg)
	if err != nil {
		log.Fatal(err)
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	return db, nil
}

func takePendingBatch(db *sql.DB, limit int) ([]Mailing, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT id, body, recipients, subject, target
		FROM mailing
		WHERE status = 'pending'
		ORDER BY created_at
		LIMIT ?
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}

	var batch []Mailing
	for rows.Next() {
		var m Mailing
		err = rows.Scan(&m.ID, &m.Body, &m.Recipients, &m.Subject, &m.Target)
		if err != nil {
			rows.Close()
			return nil, err
		}
		batch = append(batch, m)
	}
	rows.Close()

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	if len(batch) == 0 {
		return nil, tx.Commit()
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
	args := make([]interface{}, len(batch))
	for i, m := range batch {
		args[i] = m.ID
	}

	_, err = tx.Exec("UPDATE mailing SET status = 'working' WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}
	return batch, nil
}

func markFinished(db *sql.DB, id string) error {
	_, err := db.Exec("UPDATE mailing SET status = 'finished', finished_at = NOW() WHERE id = ?", id)
	return err
}

func markFailed(db *sql.DB, id string) error {
	_, err := db.Exec("UPDATE mailing SET status = 'failed' WHERE id = ?", id)
	return err
}

func resetWorking(db *sql.DB) (int64, error) {
	result, err := db.Exec("UPDATE mailing SET status = 'pending' WHERE status = 'working'")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
