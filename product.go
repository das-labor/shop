package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Product struct {
	Id          int64
	Name        string
	Slug        string
	Description string
	Price       uint64
	Count       uint64
	//	Images      []string
}

func FetchProduct(id int64, database *sql.DB) (Product, error) {
	rows, err := database.Query("SELECT * FROM products WHERE id = ?", id)

	if err != nil {
		return Product{}, err
	}

	defer rows.Close()

	if !rows.Next() {
		return Product{}, errors.New("No such product")
	}

	prod := ProductFromRow(rows)
	rows.Close()

	return prod, nil
}

func ProductFromRow(rows *sql.Rows) Product {
	var name, slug, desc string
	var id int64
	var price, count uint64

	err := rows.Scan(&id, &name, &slug, &desc, &price, &count)
	if err != nil {
		panic(err.Error())
	}
	return Product{id, name, slug, desc, price, count}
}

func NewProduct(name string, slug string, desc string, price uint64, count uint64, database *sql.DB) (Product, error) {
	prod := Product{
		Id:          0,
		Name:        name,
		Slug:        slug,
		Description: desc,
		Price:       price,
		Count:       count,
	}
	res, err := database.Exec("INSERT INTO products VALUES ( NULL, ?, ?, ?, ?, ? )", prod.Name, prod.Slug, prod.Description, prod.Price, prod.Count)

	if err != nil {
		return Product{}, err
	} else {
		var id int64
		id, err = res.LastInsertId()

		if err != nil {
			return Product{}, err
		} else {
			prod.Id = id
			return prod, nil
		}
	}
}

func GetProducts(mem Member, w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	rows, err := Database.Query("SELECT * FROM products")

	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}

	prods := make([]Product, 0)
	for rows.Next() {
		prods = append(prods, ProductFromRow(rows))
	}

	rows.Close()
	DatabaseMutex.Unlock()

	RenderTemplate(w, "products/list", "All products", mem, prods)
}

func GetProduct(prod Product, mem Member, w http.ResponseWriter, r *http.Request) {
}

func PutProduct(prod Product, mem Member, w http.ResponseWriter, r *http.Request) {
}

func DeleteProduct(prod Product, mem Member, w http.ResponseWriter, r *http.Request) {
}

func PostNewProduct(mem Member, w http.ResponseWriter, r *http.Request) {
	if mem.Group != "admin" {
		http.Error(w, "Unsufficient permissions", 403)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed create product: "+err.Error(), 500)
		return
	}

	fmt.Println("1")
	var ok bool
	var names, slugs, descs, prices, counts []string

	// Name
	names, ok = r.PostForm["name"]
	if !ok || len(names) != 1 || len(names[0]) == 0 {
		http.Error(w, "Failed create product: missing or empty name", 400)
		return
	}
	name := names[0]

	DatabaseMutex.Lock()
	defer DatabaseMutex.Unlock()

	var rows *sql.Rows
	rows, err = Database.Query("SELECT * FROM products WHERE name = ?", name)

	if err != nil {
		http.Error(w, "Failed create product: "+err.Error(), 500)
		return
	}

	exists := rows.Next()
	rows.Close()

	if exists {
		http.Error(w, "Failed create product: exists already", 400)
		return
	}

	fmt.Println("1")
	// Slug
	slugs, ok = r.PostForm["slug"]
	if !ok || len(slugs) != 1 || len(slugs[0]) == 0 {
		http.Error(w, "Failed create product: missing or empty slug", 400)
		return
	}
	slug := slugs[0]

	// Description
	descs, ok = r.PostForm["desc"]
	if !ok || len(descs) != 1 || len(descs[0]) == 0 {
		http.Error(w, "Failed create product: missing or empty desc", 400)
		return
	}
	desc := descs[0]

	// Price
	prices, ok = r.PostForm["price"]
	if !ok || len(prices) != 1 || len(prices[0]) == 0 {
		http.Error(w, "Failed create product: missing or empty price", 400)
		return
	}

	price, err := strconv.ParseUint(prices[0], 10, 64)
	if err != nil {
		http.Error(w, "Failed create product: invalid price", 400)
		return
	}

	// Count
	counts, ok = r.PostForm["count"]
	if !ok || len(counts) != 1 || len(counts[0]) == 0 {
		http.Error(w, "Failed create product: missing or empty count", 400)
		return
	}

	count, err := strconv.ParseUint(counts[0], 10, 64)
	if err != nil {
		http.Error(w, "Failed create product: invalid count", 400)
		return
	}

	fmt.Println("1")
	prod, err := NewProduct(name, slug, desc, price, count, Database)

	if err != nil {
		http.Error(w, "Failed create member: "+err.Error(), 500)
		return
	}

	fmt.Println("1")
	RenderTemplate(w, "products/single", "", mem, prod)
	fmt.Println("1")
}

func HandleProduct(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		return
	}

	fmt.Println("HandleProduct() Path = '" + r.URL.Path + "', Method = " + r.Method)

	if r.URL.Path == "/products" {
		if r.Method == "POST" {
			PostNewProduct(mem, w, r)
		} else if r.Method == "GET" {
			GetProducts(mem, w, r)
		} else {
			http.Error(w, "Method not supported", 405)
		}
	} else {
		prodId, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/products/"), 10, 64)

		if err != nil {
			http.Error(w, "Product not found: "+err.Error(), 404)
			return
		}

		DatabaseMutex.Lock()
		prod, err := FetchProduct(prodId, Database)
		DatabaseMutex.Unlock()

		if err != nil {
			http.Error(w, "Product not found: "+err.Error(), 404)
			return
		}
		if r.Method == "GET" {
			GetProduct(prod, mem, w, r)
		} else if r.Method == "PUT" {
			PutProduct(prod, mem, w, r)
		} else if r.Method == "DELETE" {
			DeleteProduct(prod, mem, w, r)
		} else {
			http.Error(w, "Not found", 404)
		}
	}
}
