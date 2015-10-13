package main

import (
	"fmt"
	"net/http"
)

func notImplHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Not Implemented yet :(")
}

func staticFileHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, r.URL.Path[1:])
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		sess := FetchOrCreateSession(w, r, Database)
		mem, err := FetchMember(sess.Member, Database)

		if err != nil {
			http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		} else {
			RenderTemplate(w, "index", "", mem, "")
		}
	} else {
		http.Error(w, "Not found", 404)
	}
}

func pagesHandler(w http.ResponseWriter, r *http.Request) {
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		return
	}

	RenderTemplate(w, r.URL.Path[1:], "", mem, "")
}

func InitializeRoutes() error {
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/pages/", pagesHandler)

	http.HandleFunc("/categories/", notImplHandler)
	http.HandleFunc("/products/", HandleProduct)

	http.HandleFunc("/orders/", HandleOrder)
	http.HandleFunc("/orders/new", HandleOrdersNew)
	http.HandleFunc("/orders/my", GetMyOrders)

	http.HandleFunc("/cart/", HandleCart)

	http.HandleFunc("/members/", HandleMember)
	http.HandleFunc("/members/login", HandleLogin)

	http.HandleFunc("/sessions/", notImplHandler)

	http.HandleFunc("/static/", staticFileHandler)

	return nil
}
