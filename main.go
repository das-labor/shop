package main

import (
	"bytes"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"html/template"
	"math"
	"net/http"
	"regexp"
	"sync"
)

type Product struct {
	Id          uint64
	Name        string
	Slug        string
	Description string
	Price       uint64
	Count       uint64
	//	Images      []string
}

func ProductFromRow(rows *sql.Rows) Product {
	var name, slug, desc string
	var id, price, count uint64

	err := rows.Scan(&id, &name, &slug, &desc, &price, &count)
	if err != nil {
		panic(err.Error())
	}
	return Product{id, name, slug, desc, price, count}
}

func RenderTemplate(w http.ResponseWriter, tmplPath string, title string, member Member, local interface{}) {
	master, err := template.ParseFiles("templates/master.html")
	global := struct {
		Title  string
		Member Member
	}{
		Title:  "Test Shop",
		Member: member,
	}

	if err != nil {
		http.Error(w, "Master template not found: "+err.Error(), 404)
	} else {
		page, err := template.ParseFiles(tmplPath)

		if err != nil {
			http.Error(w, "Template '"+tmplPath+"' not found or invalid: "+err.Error(), 500)
		} else {
			buf := new(bytes.Buffer)

			err := page.Execute(buf, local)
			if err != nil {
				http.Error(w, "Page template execution failed: "+err.Error(), 500)
			} else {
				data := struct {
					Title  string
					Body   template.HTML
					Global interface{}
					Local  interface{}
				}{
					Title:  title,
					Body:   template.HTML(buf.String()),
					Global: global,
					Local:  local,
				}

				err := master.Execute(w, data)

				if err != nil {
					http.Error(w, "Master template execution failed: "+err.Error(), 500)
				}
			}
		}
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		sess := FetchOrCreateSession(w, r, database)
		mem, err := FetchMember(sess.Member, database)

		if err != nil {
			http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		} else {
			RenderTemplate(w, "templates/index.html", "", mem, "")
		}
	} else {
		http.Error(w, "Not found", 404)
	}
}

func pagesHandler(w http.ResponseWriter, r *http.Request) {
	sess := FetchOrCreateSession(w, r, database)
	mem, err := FetchMember(sess.Member, database)

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		return
	}

	RenderTemplate(w, "templates/"+r.URL.Path[1:]+".html", "", mem, "")
}

func productHandler(w http.ResponseWriter, r *http.Request) {
	prodId := r.URL.Path[10:]

	databaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, database)
	mem, err := FetchMember(sess.Member, database)

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		databaseMutex.Unlock()
		return
	}

	if prodId == "" {
		if mem.Group == "admin" {
			rows, err := database.Query("select * from products")

			if err != nil {
				http.Error(w, "Failed to read products from database: "+err.Error(), 500)
			} else {
				defer rows.Close()

				prods := make([]Product, 0)

				for rows.Next() {
					prods = append(prods, ProductFromRow(rows))
				}

				RenderTemplate(w, "templates/products/list.html", "All products", mem, prods)
			}
		} else {
			http.Error(w, "Unsufficient permissions", 403)
		}
	} else {
		rows, err := database.Query("select * from products where id = ?", prodId)

		if err != nil {
			http.Error(w, "Failed to read product from database: "+err.Error(), 500)
		} else {
			defer rows.Close()

			if !rows.Next() {
				http.Error(w, "No such product", 404)
			} else {
				prod := ProductFromRow(rows)
				data := struct {
					Prod  Product
					Price float64
				}{
					prod,
					math.Floor(float64(prod.Price)/100.0 + 0.5),
				}
				RenderTemplate(w, "templates/products/single.html", prod.Name, mem, data)
			}
		}
	}

	databaseMutex.Unlock()
}

func memberHandler(w http.ResponseWriter, r *http.Request) {
	sess := FetchOrCreateSession(w, r, database)
	mem, err := FetchMember(sess.Member, database)

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		databaseMutex.Unlock()
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

		databaseMutex.Lock()
		defer databaseMutex.Unlock()

		var rows *sql.Rows
		rows, err = database.Query("SELECT * FROM members WHERE name = ?", name)

		if err != nil {
			http.Error(w, "Failed create member: "+err.Error(), 500)
			return
		}

		exists := rows.Next()
		rows.Close()

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

		mem2, err := NewMember(name, email, passwd, "customer", database)

		if err != nil {
			http.Error(w, "Failed create member: "+err.Error(), 500)
			return
		}

		sess := FetchOrCreateSession(w, r, database)
		err = LoginSession(sess.Id, mem2.Id, database)
		if err != nil {
			http.Error(w, "Failed to associate sessions to member: "+err.Error(), 500)
			return
		}

		RenderTemplate(w, "templates/members/success.html", "", mem2, "")
	} else if r.Method == "GET" {
		memId := r.URL.Path[9:]

		databaseMutex.Lock()
		defer databaseMutex.Unlock()

		if memId == "" {

			if mem.Group == "admin" {

				rows, err := database.Query("SELECT * FROM members")

				if err != nil {
					http.Error(w, "Failed to read members from database: "+err.Error(), 500)
				} else {
					defer rows.Close()

					mems := make([]Member, 0)

					for rows.Next() {
						mems = append(mems, MemberFromRow(rows))
					}

					RenderTemplate(w, "templates/members/list.html", "All members", mem, mems)
				}
			} else {
				http.Error(w, "Unsufficient permissions", 403)
			}
		} else {
			if memId != fmt.Sprintf("%d", mem.Id) && mem.Group != "admin" {
				http.Error(w, "Unsufficient permissions", 403)
				return
			}

			rows, err := database.Query("SELECT * FROM members WHERE id = ?", memId)

			if err != nil {
				http.Error(w, "Failed to read member from database: "+err.Error(), 500)
			} else {
				defer rows.Close()

				if !rows.Next() {
					http.Error(w, "No such member", 404)
				} else {
					mem2 := MemberFromRow(rows)
					RenderTemplate(w, "templates/members/single.html", mem.Name, mem, mem2)
				}
			}
		}
	}
}

func notImplHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Not Implemented yet :(")
}

var database *sql.DB
var databaseMutex sync.Mutex
var passwdSalt []byte
var siteTitle string

func main() {
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/pages/", pagesHandler)

	http.HandleFunc("/products/", productHandler)
	http.HandleFunc("/categories/", notImplHandler)

	http.HandleFunc("/orders/", notImplHandler)
	http.HandleFunc("/orders/new", notImplHandler)
	http.HandleFunc("/orders/my", notImplHandler)

	http.HandleFunc("/carts/", notImplHandler)
	http.HandleFunc("/carts/my", notImplHandler)

	http.HandleFunc("/members/", memberHandler)
	http.HandleFunc("/members/login", notImplHandler)

	http.HandleFunc("/sessions/", notImplHandler)

	passwdSalt = []byte("seems legit...")
	siteTitle = "Test Shop"

	fmt.Printf("Open database\n")
	var err error

	database, err = sql.Open("sqlite3", "database.db")
	if err != nil {
		fmt.Printf("Failed to open database: " + err.Error() + "\n")
	}

	_, err = database.Exec("CREATE TABLE IF NOT EXISTS products (id INTEGER PRIMARY KEY, name STRING, slug STRING, description STRING, price INTEGER, count INTEGER)")
	if err != nil {
		fmt.Printf("Failed to create product table: " + err.Error() + "\n")
	}
	_, err = database.Exec("CREATE TABLE IF NOT EXISTS sessions (id STRING PRIMARY KEY, member INTEGER, lastseen INTEGER)")
	if err != nil {
		fmt.Printf("Failed to create session table: " + err.Error() + "\n")
	}
	_, err = database.Exec("CREATE TABLE IF NOT EXISTS members (id INTEGER PRIMARY KEY, name STRING UNIQUE, email STRING, passwd STRING, grp STRING)")
	if err != nil {
		fmt.Printf("Failed to create member table: " + err.Error() + "\n")
	}
	_, err = database.Exec("INSERT OR IGNORE INTO members VALUES (0, ?, ?, ?, ?)", "Gast", "", "", "anonymous")
	if err != nil {
		fmt.Printf("Failed to create guest member: " + err.Error() + "\n")
	}

	fmt.Printf("Listen on 127.0.0.1:8080\n")
	http.ListenAndServe(":8080", nil)
}
