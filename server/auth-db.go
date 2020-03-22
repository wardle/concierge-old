package server

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/lib/pq"

	"github.com/wardle/concierge/apiv1"
	"golang.org/x/crypto/bcrypt"
)

type dbAuthProvider struct {
	db *sql.DB
}

// NewDatabaseAuthProvider is an auth provider that uses a PostgreSQL database to validate credentials
func NewDatabaseAuthProvider(connStr string) (AuthProvider, error) {
	for {
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			goto dberror
		}
		err = db.Ping()
		if err != nil {
			goto dberror
		}
		return &dbAuthProvider{
			db: db,
		}, nil
	dberror:
		log.Println(err)
		log.Println("auth: error connecting to the authentication database, retrying in 5 secs.")
		time.Sleep(5 * time.Second)
	}
}

func (dba *dbAuthProvider) Authenticate(id *apiv1.Identifier, credential string) (bool, error) {
	rows, err := dba.db.Query("SELECT password FROM users WHERE username=$1", id.GetValue())
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var hash string
	for rows.Next() {
		if err := rows.Scan(&hash); err != nil {
			return false, err
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(credential)); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	log.Printf("auth: no user found matching %s|%s", id.GetSystem(), id.GetValue())
	return false, nil
}
