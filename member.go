package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"golang.org/x/crypto/pbkdf2"
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
	hashed := pbkdf2.Key([]byte(passwd), passwdSalt, 8192, 32, sha256.New)

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

	defer rows.Close()

	if !rows.Next() {
		return Member{}, errors.New("No such member")
	}

	mem := MemberFromRow(rows)
	rows.Close()

	return mem, nil
}
