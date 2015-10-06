package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Session struct {
	Id       string
	Member   int64
	LastSeen int64 // Unix time
}

func SessionFromRow(rows *sql.Rows) Session {
	var id string
	var mem, time int64

	err := rows.Scan(&id, &mem, &time)
	if err != nil {
		panic(err.Error())
	}
	return Session{id, mem, time}
}

func NewSession(database *sql.DB) (Session, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)

	if err != nil {
		return Session{}, err
	}

	sess := Session{hex.EncodeToString(id), 0, time.Now().Unix()}
	_, err = database.Exec("INSERT INTO sessions VALUES ( ?, ?, ? )", sess.Id, sess.Member, sess.LastSeen)

	if err != nil {
		return Session{}, err
	} else {
		return sess, nil
	}
}

func LoginSession(sess string, mem int64, database *sql.DB) error {
	res, err := database.Exec("UPDATE sessions SET member = ? WHERE id = ?", mem, sess)

	if err != nil {
		return err
	} else {
		cnt, err := res.RowsAffected()
		if err != nil {
			return err
		} else if cnt != 1 {
			return errors.New("Invalid affected row count: " + fmt.Sprintf("%d", cnt))
		} else {
			return nil
		}
	}
}

func RefreshSession(id string, database *sql.DB) (Session, error) {
	rows, err := database.Query("SELECT * FROM sessions WHERE id = ?", id)

	if err != nil {
		return Session{}, err
	}

	if !rows.Next() {
		return Session{}, errors.New("No such session")
	}

	sess := SessionFromRow(rows)
	rows.Close()

	if time.Now().Unix()-sess.LastSeen > 3*60*60*24 {
		return NewSession(database)
	} else {
		_, err = database.Exec("UPDATE sessions SET lastseen = ? WHERE id = ?", time.Now().Unix(), sess.Id)

		return sess, err
	}
}

func NewCookie(name string, value string, exp time.Time) http.Cookie {
	return http.Cookie{
		Name:    name,
		Value:   value,
		Path:    GlobalConfig.Location + "/",
		Domain:  GlobalConfig.CookieDomain,
		Expires: exp,
		Secure:  GlobalConfig.CookieSecure,
	}
}

func FetchOrCreateSession(w http.ResponseWriter, r *http.Request, database *sql.DB) Session {
	c, err := r.Cookie("sessid")
	var sess Session

	if err != nil {
		if err != http.ErrNoCookie {
			log.Panicln(err)
		}

		sess, err = NewSession(database)

		if err != nil {
			panic(err.Error())
		}
	} else if err == nil {
		sess, err = RefreshSession(c.Value, database)

		if err != nil {
			sess, err = NewSession(database)

			if err != nil {
				log.Panicln(err.Error())
			}
		}
	}

	coo := NewCookie("sessid", sess.Id, time.Now().Add(time.Duration(3*24)*time.Hour))
	http.SetCookie(w, &coo)
	return sess
}
