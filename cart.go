package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type CartItem struct {
	Product    Product
	Amount     uint64
	NextAmount uint64
	PrevAmount uint64
}

func AddToCart(form url.Values, member Member, session Session, w http.ResponseWriter, r *http.Request) {
	// Product Id
	ids, ok := form["id"]
	if !ok || len(ids) != 1 || len(ids[0]) == 0 {
		http.Error(w, "Missing or empty id", 400)
		return
	}
	id := ids[0]

	// Count
	counts, ok := form["count"]
	if !ok || len(counts) != 1 || len(counts[0]) == 0 {
		http.Error(w, "Missing or empty count", 400)
		return
	}

	count, err := strconv.ParseUint(counts[0], 10, 64)
	if err != nil || count == 0 {
		http.Error(w, "Invalid count", 400)
		return
	}

	DatabaseMutex.Lock()

	tx, err := Database.Begin()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows, err := tx.Query("SELECT count FROM products WHERE id = ?", id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	if !rows.Next() {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: no such cart", 500)
		return
	}

	var avail_count uint64
	err = rows.Scan(&avail_count)
	if err != nil {
		rows.Close()
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows.Close()

	if avail_count < count {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: no enough items in stock", 400)
		return
	}

	_, err = tx.Exec("INSERT INTO carts VALUES ( ?, ?, ? )", id, session.Id, count)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	res, err := tx.Exec("UPDATE products SET count = count - ? WHERE id = ?", count, id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	err = tx.Commit()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	ar, err := res.RowsAffected()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	if ar != 1 {
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/cart", 301)
}

func GetCart(member Member, session Session, w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()

	var rows *sql.Rows
	rows, err := Database.Query("SELECT products.id,products.name,products.slug,products.description,products.price,products.count,carts.count as selected_count FROM carts JOIN products ON products.id = carts.product WHERE session = ?", session.Id)

	if err != nil {
		http.Error(w, "Failed fetch cart: "+err.Error(), 500)
		return
	}

	cart := make([]CartItem, 0)
	for rows.Next() {
		var name, slug, desc string
		var id int64
		var price, count, amount uint64

		err := rows.Scan(&id, &name, &slug, &desc, &price, &count, &amount)
		prod := Product{Id: id, Name: name, Slug: slug, Description: desc, Price: price, Count: count}
		itm := CartItem{Product: prod, Amount: amount, NextAmount: amount + 1, PrevAmount: amount - 1}

		if err == nil {
			cart = append(cart, itm)
		}
	}

	DatabaseMutex.Unlock()

	RenderTemplate(w, "cart", "", member, cart)
}

func PutCartItem(prodId int64, member Member, session Session, w http.ResponseWriter, r *http.Request) {
	// Amount
	counts, ok := r.PostForm["count"]
	if !ok || len(counts) != 1 || len(counts[0]) == 0 {
		http.Error(w, "Missing or empty count", 400)
		return
	}

	count, err := strconv.ParseUint(counts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid count", 400)
		return
	}

	if count == 0 {
		DeleteCartItem(prodId, member, session, w, r)
		return
	}

	DatabaseMutex.Lock()

	tx, err := Database.Begin()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows, err := tx.Query("SELECT count FROM carts WHERE product = ? AND session = ?", prodId, session.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	if !rows.Next() {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: no such cart", 500)
		return
	}

	var cur_count uint64
	err = rows.Scan(&cur_count)
	if err != nil {
		rows.Close()
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows.Close()

	rows, err = tx.Query("SELECT count FROM products WHERE id = ?", prodId)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	if !rows.Next() {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: no such cart", 500)
		return
	}

	var avail_count uint64
	err = rows.Scan(&avail_count)
	if err != nil {
		rows.Close()
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows.Close()

	if avail_count+cur_count < count {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: no enough items in stock", 400)
		return
	}

	_, err = tx.Exec("UPDATE products SET count = count + ? WHERE id = ?", cur_count, prodId)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	_, err = tx.Exec("UPDATE carts SET count = ? WHERE product = ? AND session = ?", count, prodId, session.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	_, err = tx.Exec("UPDATE products SET count = count - ? WHERE id = ?", count, prodId)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	err = tx.Commit()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Unlock()

	http.Redirect(w, r, "/cart", 301)
}

func DeleteCartItem(prodId int64, member Member, session Session, w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()

	tx, err := Database.Begin()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows, err := tx.Query("SELECT count FROM carts WHERE product = ? AND session = ?", prodId, session.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	if !rows.Next() {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: no such cart", 500)
		return
	}

	var cur_count uint
	err = rows.Scan(&cur_count)
	if err != nil {
		rows.Close()
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	rows.Close()

	_, err = tx.Exec("UPDATE products SET count = count + ? WHERE id = ?", cur_count, prodId)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	_, err = tx.Exec("DELETE FROM carts WHERE product = ? AND session = ?", prodId, session.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	err = tx.Commit()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Unlock()
	http.Redirect(w, r, "/cart", 301)
}

func HandleCart(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		return
	}

	fmt.Println("HandleCart() Path = '" + r.URL.Path + "', Method = " + r.Method)

	if r.URL.Path == "/cart" || r.URL.Path == "/cart/" {
		if r.Method == "POST" {
			err := r.ParseForm()

			if err != nil {
				http.Error(w, "Failed parse <form>: "+err.Error(), 500)
				return
			}

			AddToCart(r.PostForm, mem, sess, w, r)
		} else if r.Method == "GET" {
			GetCart(mem, sess, w, r)
		} else {
			http.Error(w, "Method not supported", 405)
		}
	} else {
		prodId, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/cart/"), 10, 64)

		if err != nil {
			http.Error(w, "Product not found in cart: "+err.Error(), 404)
			return
		}

		if r.Method == "POST" {
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
				PutCartItem(prodId, mem, sess, w, r)
			} else if meth == "DELETE" {
				DeleteCartItem(prodId, mem, sess, w, r)
			} else {
				http.Error(w, "Method not supported", 405)
			}
		} else {
			http.Error(w, "Not found", 404)
		}
	}
}
