package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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

func HashPassword(passwd string) string {
	return hex.EncodeToString(pbkdf2.Key([]byte(passwd), []byte(GlobalConfig.Salt), 8192, 32, sha256.New))
}

func NewMember(name string, email string, passwd string, group string, database *sql.DB) (Member, error) {
	mem := Member{
		Id:     0,
		Name:   name,
		EMail:  email,
		Passwd: HashPassword(passwd),
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

func MemberFromForm(form url.Values) (Member, error) {
	var ok bool
	var names, emails, passwds, groups []string
	var name, email, passwd, group string

	names, ok = form["name"]
	if !ok || len(names) != 1 || names[0] == "" {
		return Member{}, fmt.Errorf("No name given")
	} else {
		name = names[0]
	}

	emails, ok = form["email"]
	if !ok || len(emails) != 1 {
		return Member{}, fmt.Errorf("No email given")
	} else {
		email = emails[0]
		matched, err := regexp.MatchString(".+@.+", email)

		if err != nil || !matched {
			return Member{}, fmt.Errorf("Not an email address")
		}
	}

	passwds, ok = form["passwd"]
	if ok && len(passwds) == 1 && len(passwds[0]) >= 8 {
		passwd = passwds[0]
	} else {
		passwd = ""
	}

	groups, ok = form["group"]
	if ok && len(groups) == 1 && (groups[0] == "admin" || groups[0] == "customer") {
		group = groups[0]
	} else {
		group = "customer"
	}

	return Member{
		Id:     0,
		Name:   name,
		EMail:  email,
		Passwd: HashPassword(passwd),
		Group:  group,
	}, nil
}

func PostNewMember(cur_mem Member, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()

	if err != nil {
		http.Error(w, "Failed create member: "+err.Error(), 500)
	}

	new_mem, err := MemberFromForm(r.PostForm)

	if err != nil {
		http.Error(w, "Failed create member: "+err.Error(), 400)
		return
	}

	DatabaseMutex.Lock()

	var rows *sql.Rows
	rows, err = Database.Query("SELECT * FROM members WHERE name = ?", new_mem.Name)

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

	DatabaseMutex.Lock()
	mem2, err := NewMember(new_mem.Name, new_mem.EMail, new_mem.Passwd, "customer", Database)

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
}

func ResetPasswd(mem Member, cur_mem Member, w http.ResponseWriter, r *http.Request) {
	if cur_mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to update member: "+err.Error(), 500)
		return
	}

	passwds, ok := r.PostForm["passwd"]
	if ok && len(passwds) == 1 && len(passwds[0]) >= 8 {
		DatabaseMutex.Lock()
		_, err = Database.Exec("UPDATE members SET passwd = ? WHERE id = ?", HashPassword(passwds[0]), mem.Id)
		DatabaseMutex.Unlock()

		http.Redirect(w, r, "/members/"+strconv.FormatInt(mem.Id, 10), 301)
	} else {
		http.Error(w, "Failed to reset password: passwords must be 8 characters or longer", 500)
	}
}

func PutMember(mem Member, cur_mem Member, w http.ResponseWriter, r *http.Request) {
	if cur_mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to update member: "+err.Error(), 500)
		return
	}

	new_mem, err := MemberFromForm(r.PostForm)

	if err != nil {
		http.Error(w, "Failed to update member: "+err.Error(), 400)
		return
	}

	DatabaseMutex.Lock()

	var rows *sql.Rows
	rows, err = Database.Query("SELECT * FROM members WHERE name = ? AND id <> ?", new_mem.Name, mem.Id)

	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to update member: "+err.Error(), 500)
		return
	}

	exists := rows.Next()
	rows.Close()
	DatabaseMutex.Unlock()

	if exists {
		http.Error(w, "Failed to update member: name exists already", 400)
		return
	}

	DatabaseMutex.Lock()
	_, err = Database.Exec("UPDATE members SET name = ?, email = ?, grp = ? WHERE id = ?", new_mem.Name, new_mem.EMail, new_mem.Group, mem.Id)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to update member: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/members/"+strconv.FormatInt(mem.Id, 10), 301)
}

func DeleteMember(mem Member, cur_mem Member, w http.ResponseWriter, r *http.Request) {
	if cur_mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	DatabaseMutex.Lock()
	rows, err := Database.Query("DELETE FROM members WHERE id = ?", mem.Id)

	if err != nil {
		http.Error(w, "Failed delete member: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	exists := rows.Next()
	rows.Close()
	DatabaseMutex.Unlock()

	if exists {
		http.Error(w, "Failed delete member: does not exists", 400)
		return
	}

	http.Redirect(w, r, "/members/", 301)
}

func GetMembers(mem Member, w http.ResponseWriter, r *http.Request) {
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
	DatabaseMutex.Unlock()

	RenderTemplate(w, "members/list", "", mem, mems)
}

func GetMember(mem Member, cur_mem Member, w http.ResponseWriter, r *http.Request) {
	if cur_mem.Group != "admin" {
		http.Error(w, "Unsufficient permissions", 403)
		return
	}

	DatabaseMutex.Lock()
	rows, err := Database.Query("SELECT * FROM members WHERE id = ?", mem.Id)

	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to read member from database: "+err.Error(), 500)
		return
	}

	if !rows.Next() {
		DatabaseMutex.Unlock()
		http.Error(w, "No such member", 404)
	}

	mem2 := MemberFromRow(rows)
	rows.Close()
	DatabaseMutex.Unlock()
	RenderTemplate(w, "members/single", mem.Name, cur_mem, mem2)
}

func HandleMember(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	cur_mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	fmt.Println("HandleMember() Path = '" + r.URL.Path + "', Method = " + r.Method)

	if r.URL.Path == "/members" || r.URL.Path == "/members/" {
		if r.Method == "POST" {
			PostNewMember(cur_mem, w, r)
		} else if r.Method == "GET" {
			GetMembers(cur_mem, w, r)
		} else {
			http.Error(w, "Method not supported", 405)
		}
	} else {
		suff := strings.TrimPrefix(r.URL.Path, "/members/")
		matched, err := regexp.MatchString("[0-9]+/passwd/?", suff)

		if err == nil && matched && r.Method == "POST" {
			fmt.Println("in")
			memId, err := strconv.ParseInt(strings.TrimSuffix(suff, "/passwd"), 10, 64)

			if err != nil {
				http.Error(w, "Member not found: "+err.Error(), 404)
				return
			}

			DatabaseMutex.Lock()
			mem2, err := FetchMember(memId, Database)
			DatabaseMutex.Unlock()

			if err != nil {
				http.Error(w, "Member not found: "+err.Error(), 404)
				return
			}

			ResetPasswd(mem2, cur_mem, w, r)
		} else {
			memId, err := strconv.ParseInt(suff, 10, 64)

			if err != nil {
				http.Error(w, "Member not found: "+err.Error(), 404)
				return
			}

			DatabaseMutex.Lock()
			mem2, err := FetchMember(memId, Database)
			DatabaseMutex.Unlock()

			if err != nil {
				http.Error(w, "Member not found: "+err.Error(), 404)
				return
			}

			if r.Method == "POST" {
				if r.ParseForm() != nil {
					http.Error(w, "Member not found: "+err.Error(), 500)
				}

				meth := "POST"
				meths, ok := r.PostForm["_method"]
				if ok && len(meths) == 1 {
					meth = meths[0]
				}

				if meth == "PUT" {
					PutMember(mem2, cur_mem, w, r)
				} else if meth == "DELETE" {
					DeleteMember(mem2, cur_mem, w, r)
				}
			} else if r.Method == "GET" {
				GetMember(mem2, cur_mem, w, r)
			} else {
				http.Error(w, "Method not supported", 405)
			}
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
			http.Error(w, "Invalid username or password", 500)
			log.Print("Login: can't parse name field")
			return
		}

		name := names[0]

		passwds, ok = r.PostForm["passwd"]
		if !ok || len(passwds) != 1 {
			http.Error(w, "Invalid username or password", 500)
			log.Print("Login: can't parse passwd field")
			return
		}

		passwd := passwds[0]
		hashed := pbkdf2.Key([]byte(passwd), []byte(GlobalConfig.Salt), 8192, 32, sha256.New)

		DatabaseMutex.Lock()

		var rows *sql.Rows
		rows, err = Database.Query("SELECT id FROM members WHERE name = ? AND passwd = ?", name, hex.EncodeToString(hashed))

		if err != nil {
			DatabaseMutex.Unlock()
			http.Error(w, "Invalid username or password", 500)
			log.Print("Login: SELECT failed (" + err.Error() + ")")
			return
		}

		exists := rows.Next()

		if !exists {
			DatabaseMutex.Unlock()
			http.Error(w, "Invalid username or password", 500)
			log.Print("Login: SELECT did not return anything")
			return
		}

		var memid int64
		err = rows.Scan(&memid)
		rows.Close()

		if err != nil {
			DatabaseMutex.Unlock()
			http.Error(w, "Invalid username or password", 500)
			log.Print("Login: can't parse SELECT")
			return
		}

		err = LoginSession(sess.Id, memid, Database)
		if err != nil {
			DatabaseMutex.Unlock()
			http.Error(w, "Invalid username or password", 500)
			log.Print("Login: can't login session (" + err.Error() + ")")
			return
		}

		DatabaseMutex.Unlock()
		RenderTemplate(w, "members/success", "", mem, "")
	} else {
		http.Error(w, "Method not supported", 405)
	}
}
