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

	//http.Handle("/products/", CrudHandler{PrettyName: "Product", Table: "products", Subdir: "product"})
	http.HandleFunc("/categories/", notImplHandler)

	http.HandleFunc("/orders/", notImplHandler)
	http.HandleFunc("/orders/new", notImplHandler)
	http.HandleFunc("/orders/my", notImplHandler)

	/*cartHandler := CrudHandler{
		PrettyName: "Cart",
		Table:      "carts",
		Subdir:     "cart",
	}

	http.Handle("/carts/", cartHandler)*/
	http.HandleFunc("/carts/my", notImplHandler)

	http.HandleFunc("/members/", HandleMember)
	http.HandleFunc("/members/login", HandleLogin)

	http.HandleFunc("/sessions/", notImplHandler)

	http.HandleFunc("/static/", staticFileHandler)

	return nil
}
