package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"net/http"
	"regexp"
)

type Member struct {
	Id     int64
	Name   string
	EMail  string
	Passwd string
	Group  string
}

func MemberFromRow(rows *sql.Rows) Member {
	var name, email, passwd, group string
	var id int64

	err := rows.Scan(&id, &name, &email, &passwd, &group)
	if err != nil {
		panic(err.Error())
	}
	return Member{
		Id:     id,
		Name:   name,
		EMail:  email,
		Passwd: passwd,
		Group:  group,
	}
}

func NewMember(name string, email string, passwd string, group string, database *sql.DB) (Member, error) {
	hashed := pbkdf2.Key([]byte(passwd), []byte(GlobalConfig.Salt), 8192, 32, sha256.New)

	mem := Member{
		Id:     0,
		Name:   name,
		EMail:  email,
		Passwd: hex.EncodeToString(hashed),
		Group:  group,
	}
	res, err := database.Exec("INSERT INTO members VALUES ( NULL, ?, ?, ?, ? )", mem.Name, mem.EMail, mem.Passwd, mem.Group)

	if err != nil {
		return Member{}, err
	} else {
		var id int64
		id, err = res.LastInsertId()

		if err != nil {
			return Member{}, err
		} else {
			mem.Id = id
			return mem, nil
		}
	}
}

func FetchMember(id int64, database *sql.DB) (Member, error) {
	rows, err := database.Query("SELECT * FROM members WHERE id = ?", id)

	if err != nil {
		return Member{}, err
	}

	if !rows.Next() {
		return Member{}, errors.New("No such member")
	}

	mem := MemberFromRow(rows)
	rows.Close()

	return mem, nil
}

func HandleMember(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	if r.Method == "POST" {
		err := r.ParseForm()

		if err != nil {
			http.Error(w, "Failed create member: "+err.Error(), 500)
		}

		var ok bool
		var names, emails, passwds []string

		names, ok = r.PostForm["name"]
		if !ok || len(names) != 1 {
			http.Error(w, "Failed create member: missing name", 400)
			return
		}

		name := names[0]
		if name == "" {
			http.Error(w, "Failed create member: empty name", 400)
			return
		}

		DatabaseMutex.Lock()

		var rows *sql.Rows
		rows, err = Database.Query("SELECT * FROM members WHERE name = ?", name)

		if err != nil {
			DatabaseMutex.Unlock()
			http.Error(w, "Failed create member: "+err.Error(), 500)
			return
		}

		exists := rows.Next()
		rows.Close()
		DatabaseMutex.Unlock()

		if exists {
			http.Error(w, "Failed create member: exists already", 400)
			return
		}

		emails, ok = r.PostForm["email"]
		if !ok || len(emails) != 1 {
			http.Error(w, "Failed create member: missing email", 400)
			return
		}

		email := emails[0]
		var matched bool
		matched, err = regexp.MatchString(".+@.+", email)
		if !matched || err != nil {
			http.Error(w, "Failed create member: invalid email", 400)
			return
		}

		passwds, ok = r.PostForm["passwd"]
		if !ok || len(passwds) != 1 {
			http.Error(w, "Failed create member: missing passwd", 400)
			return
		}

		passwd := passwds[0]
		if len(passwd) < 8 {
			http.Error(w, "Failed create member: password shorter than 8 characters", 400)
			return
		}

		DatabaseMutex.Lock()
		mem2, err := NewMember(name, email, passwd, "customer", Database)

		if err != nil {
			DatabaseMutex.Unlock()
			http.Error(w, "Failed create member: "+err.Error(), 500)
			return
		}

		sess := FetchOrCreateSession(w, r, Database)
		err = LoginSession(sess.Id, mem2.Id, Database)

		DatabaseMutex.Unlock()

		if err != nil {
			http.Error(w, "Failed to associate sessions to member: "+err.Error(), 500)
			return
		}

		RenderTemplate(w, "members/success", "", mem2, "")
	} else if r.Method == "GET" {
		memId := r.URL.Path[9:]

		if memId == "" {
			if mem.Group != "admin" {
				http.Error(w, "Unsufficient permissions", 403)
				return
			}

			DatabaseMutex.Lock()
			rows, err := Database.Query("SELECT * FROM members")

			if err != nil {
				DatabaseMutex.Unlock()
				http.Error(w, "Failed to read members from database: "+err.Error(), 500)
				return
			}

			mems := make([]Member, 0)

			for rows.Next() {
				mems = append(mems, MemberFromRow(rows))
			}
			rows.Close()
			DatabaseMutex.Unlock()

			RenderTemplate(w, "members/list", "All members", mem, mems)
		} else {
			if memId != fmt.Sprintf("%d", mem.Id) && mem.Group != "admin" {
				http.Error(w, "Unsufficient permissions", 403)
				return
			}

			DatabaseMutex.Lock()
			rows, err := Database.Query("SELECT * FROM members WHERE id = ?", memId)

			if err != nil {
				DatabaseMutex.Unlock()
				http.Error(w, "Failed to read member from database: "+err.Error(), 500)
				return
			}

			if !rows.Next() {
				rows.Close()
				DatabaseMutex.Unlock()
				http.Error(w, "No such member", 404)
			}

			mem2 := MemberFromRow(rows)
			rows.Close()
			DatabaseMutex.Unlock()
			RenderTemplate(w, "members/single", mem.Name, mem, mem2)
		}
	}
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	if r.Method == "POST" {
		err := r.ParseForm()

		if err != nil {
			http.Error(w, "Failed parse <form>: "+err.Error(), 500)
		}

		var ok bool
		var names, passwds []string

		names, ok = r.PostForm["name"]
		if !ok || len(names) != 1 {
			RenderTemplate(w, "pages/login", "", mem, "Invalid username or password.1")
			return
		}

		name := names[0]

		passwds, ok = r.PostForm["passwd"]
		if !ok || len(passwds) != 1 {
			RenderTemplate(w, "pages/login", "", mem, "Invalid username or password.2")
			return
		}

		passwd := passwds[0]
		hashed := pbkdf2.Key([]byte(passwd), []byte(GlobalConfig.Salt), 8192, 32, sha256.New)

		DatabaseMutex.Lock()

		var rows *sql.Rows
		rows, err = Database.Query("SELECT id FROM members WHERE name = ? AND passwd = ?", name, hex.EncodeToString(hashed))

		if err != nil {
			DatabaseMutex.Unlock()
			RenderTemplate(w, "pages/login", "", mem, "Invalid username or password.3")
			return
		}

		exists := rows.Next()

		if !exists {
			DatabaseMutex.Unlock()
			RenderTemplate(w, "pages/login", "", mem, "Invalid username or password.4")
			return
		}

		var memid int64
		err = rows.Scan(&memid)
		rows.Close()

		if err != nil {
			DatabaseMutex.Unlock()
			RenderTemplate(w, "pages/login", "", mem, "Invalid username or password.5")
			return
		}

		err = LoginSession(sess.Id, memid, Database)
		if err != nil {
			DatabaseMutex.Unlock()
			RenderTemplate(w, "pages/login", "", mem, "Invalid username or password.6")
			return
		}

		DatabaseMutex.Unlock()
		RenderTemplate(w, "members/success", "", mem, "")
	} else {
		RenderTemplate(w, "pages/login", "", mem, "Invalid username or password. (GET)")
	}
}
