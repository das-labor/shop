package main

import (
	"net/http"
)

type CrudHandler struct {
	PrettyName string
	Subdir     string
	Table      string
	FromRow    func(*sql.Rows)
}

func (handler *CrudHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len(handler.PrettyName):]
	sess := FetchOrCreateSession(w, r, database)
	mem, err := FetchMember(sess.Member, database)

	if err != nil {
		return handler.InternalError("Failed to fetch member: "+err.Error(), w, r)
	}

	if r.Method == "GET" {
		if id == "" {
			if mem.Group == "admin" {
				return handler.ServeList(mem, w, r)
			} else {
				return handler.PermError(w, r)
			}
		} else {
			return handler.ServeGet(mem, id, w, r)
		}
	} else if r.Method == "POST" {
		if id == "" {
			if mem.Group == "admin" {
				return handler.ServePost(mem, w, r)
			} else {
				return handler.PermError(w, r)
			}
		} else {
			return handler.NotFoundError(w, r)
		}
	} else if r.Method == "PUT" {
		if id != "" {
			if mem.Group == "admin" {
				return handler.ServePut(id, mem, w, r)
			} else {
				return handler.PermError(w, r)
			}
		} else {
			return handler.NotFoundError(w, r)
		}
	} else if r.Method == "DELETE" {
		if id != "" {
			if mem.Group == "admin" {
				return handler.ServeDelete(id, mem, w, r)
			} else {
				return handler.PermError(w, r)
			}
		} else {
			return handler.NotFoundError(w, r)
		}
	}
}

func (handler *CrudHandler) ServeList(mem Member, w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query("select * from " + handler.Table)

	if err != nil {
		return handler.InternalError("Failed to read "+handler.PrettyName+" from database: "+err.Error(), w, r)
	} else {
		defer rows.Close()

		elem := make([]interface{}, 0)

		for rows.Next() {
			elem = append(elem, handler.FromRow(rows))
		}

		RenderTemplate(w, "templates/"+handler.Subdir+"/list.html", "All "+handler.PrettyName, mem, elem)
	}
}

func (handler *CrudHandler) ServeGet(id string, mem Member, w http.ResponseWriter, r *http.Request) {
	rows, err := database.Query("select * from "+handler.Table+" where id = ?", id)

	if err != nil {
		handler.InternalError("Failed to read product from database: "+err.Error(), w, r)
	} else {
		defer rows.Close()

		if !rows.Next() {
			handler.NotFoundError(wmr)
		} else {
			elem := handler.FromRow(rows)
			RenderTemplate(w, "templates/"+handler.Subdir+"/single.html", handler.PrettyName, mem, elem)
		}
	}
}

func (handler *CrudHandler) ServePut(id string, mem Member, w http.ResponseWriter, r *http.Request) {
	http.Error(w, "PUT not implemented", 500)
}

func (handler *CrudHandler) ServePost(mem Member, w http.ResponseWriter, r *http.Request) {
	http.Error(w, "POST not implemented", 500)
}
func (handler *CrudHandler) PermError(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Insufficient Permissions", 500)
}
func (handler *CrudHandler) InternalError(msg string, w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Internal Error", 500)
}
func (handler *CrudHandler) NotFoundError(mem Member, w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Found", 404)
}
