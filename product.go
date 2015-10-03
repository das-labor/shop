package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

	prod, err := ProductFromRow(rows)
	rows.Close()

	return prod, err
}

func InsertProduct(prod Product, database *sql.DB) (Product, error) {
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

func UpdateProduct(prod Product, database *sql.DB) (Product, error) {
	res, err := database.Exec("UPDATE products SET name = ?, slug = ?, description = ?, price = ?, count = ? WHERE id = ?",
		prod.Name, prod.Slug, prod.Description, prod.Price, prod.Count, prod.Id)

	if err != nil {
		return Product{}, err
	} else {
		rows, err := res.RowsAffected()

		if err != nil {
			return Product{}, err
		}

		if rows == 0 {
			return Product{}, fmt.Errorf("Product not found")
		}

		if rows > 1 {
			return Product{}, fmt.Errorf("Product in database more than once")
		}

		return prod, nil
	}
}

func ProductFromRow(rows *sql.Rows) (Product, error) {
	var name, slug, desc string
	var id int64
	var price, count uint64

	err := rows.Scan(&id, &name, &slug, &desc, &price, &count)
	if err != nil {
		return Product{}, err
	}
	return Product{id, name, slug, desc, price, count}, nil
}

func ProductFromForm(form url.Values) (Product, error) {
	var ok bool
	var names, slugs, descs, prices, counts []string
	var ret Product

	// Name
	names, ok = form["name"]
	if !ok || len(names) != 1 || len(names[0]) == 0 {
		return ret, fmt.Errorf("Missing or empty name")
	}
	name := names[0]

	// Slug
	slugs, ok = form["slug"]
	if !ok || len(slugs) != 1 || len(slugs[0]) == 0 {
		return ret, fmt.Errorf("Missing or empty slug")
	}
	slug := slugs[0]

	// Description
	descs, ok = form["desc"]
	if !ok || len(descs) != 1 || len(descs[0]) == 0 {
		return ret, fmt.Errorf("Missing or empty desc")
	}
	desc := descs[0]

	// Price
	prices, ok = form["price"]
	if !ok || len(prices) != 1 || len(prices[0]) == 0 {
		return ret, fmt.Errorf("Missing or empty price")
	}

	price, err := strconv.ParseUint(prices[0], 10, 64)
	if err != nil {
		return ret, fmt.Errorf("Invalid price")
	}

	// Count
	counts, ok = form["count"]
	if !ok || len(counts) != 1 || len(counts[0]) == 0 {
		return ret, fmt.Errorf("Missing or empty count")
	}

	count, err := strconv.ParseUint(counts[0], 10, 64)
	if err != nil {
		return ret, fmt.Errorf("Invalid count")
	}

	ret = Product{
		Id:          0,
		Name:        name,
		Slug:        slug,
		Description: desc,
		Price:       price,
		Count:       count,
	}

	return ret, nil
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
		prod, err := ProductFromRow(rows)

		if err == nil {
			prods = append(prods, prod)
		}
	}

	rows.Close()
	DatabaseMutex.Unlock()

	RenderTemplate(w, "products/list", "All products", mem, prods)
}

func GetProduct(prod Product, mem Member, w http.ResponseWriter, r *http.Request) {
	RenderTemplate(w, "products/single", "", mem, prod)
}

func PutProduct(prod Product, mem Member, w http.ResponseWriter, r *http.Request) {
	if mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to update product: "+err.Error(), 500)
		return
	}

	new_prod, err := ProductFromForm(r.PostForm)

	if err != nil {
		http.Error(w, "Failed to parse product form: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Lock()
	var rows *sql.Rows
	rows, err = Database.Query("SELECT * FROM products WHERE name = ? AND id <> ?", new_prod.Name, prod.Id)

	if err != nil {
		http.Error(w, "Failed to update product: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	exists := rows.Next()
	rows.Close()
	DatabaseMutex.Unlock()

	if exists {
		http.Error(w, "Failed to update product: exists already", 400)
		return
	}

	DatabaseMutex.Lock()
	new_prod.Id = prod.Id
	prod, err = UpdateProduct(new_prod, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to parse product form: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/products/"+strconv.FormatInt(prod.Id, 10), 301)
}

func DeleteProduct(prod Product, mem Member, w http.ResponseWriter, r *http.Request) {
	if mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	DatabaseMutex.Lock()
	rows, err := Database.Query("DELETE FROM products WHERE id = ?", prod.Id)

	if err != nil {
		http.Error(w, "Failed delete product: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	exists := rows.Next()
	rows.Close()
	DatabaseMutex.Unlock()

	if exists {
		http.Error(w, "Failed delete product: does not exists", 400)
		return
	}

	http.Redirect(w, r, "/products/", 301)
}

func PostNewProduct(mem Member, w http.ResponseWriter, r *http.Request) {
	if mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	prod, err := ProductFromForm(r.PostForm)

	if err != nil {
		http.Error(w, "Failed to parse product form: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Lock()
	var rows *sql.Rows
	rows, err = Database.Query("SELECT * FROM products WHERE name = ?", prod.Name)

	if err != nil {
		http.Error(w, "Failed create product: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	exists := rows.Next()
	rows.Close()
	DatabaseMutex.Unlock()

	if exists {
		http.Error(w, "Failed create product: exists already", 400)
		return
	}

	DatabaseMutex.Lock()
	prod, err = InsertProduct(prod, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to parse product form: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/products/"+strconv.FormatInt(prod.Id, 10), 301)
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

	if r.URL.Path == "/products" || r.URL.Path == "/products/" {
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
		} else if r.Method == "POST" {
			err := r.ParseForm()

			if err != nil {
				http.Error(w, "Failed to parse form data: "+err.Error(), 500)
				return
			}

			var meth string
			meths, ok := r.PostForm["_method"]
			if !ok || len(meths) != 1 || len(meths[0]) == 0 {
				meth = "POST"
			} else {
				meth = meths[0]
			}

			if meth == "PUT" {
				PutProduct(prod, mem, w, r)
			} else if meth == "DELETE" {
				DeleteProduct(prod, mem, w, r)
			} else {
				http.Error(w, "Not found", 404)
			}
		} else {
			http.Error(w, "Not found", 404)
		}
	}
}
