package main

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/pborman/uuid"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Order struct {
	Id     int64
	Date   int64
	Member int64
	Status string
	Uuid   string
}

func OrderFromRow(rows *sql.Rows) (Order, error) {
	var status, uuid string
	var id, mem, date int64

	err := rows.Scan(&id, &date, &mem, &status, &uuid)
	if err != nil {
		return Order{}, err
	}
	return Order{
		Id:     id,
		Date:   date,
		Member: mem,
		Status: status,
		Uuid:   uuid,
	}, nil
}

func NewOrder(member Member, uuid string, tx *sql.Tx) (Order, error) {
	ord := Order{
		Id:     0,
		Date:   time.Now().Unix(),
		Member: member.Id,
		Status: "new",
		Uuid:   uuid,
	}
	res, err := tx.Exec("INSERT INTO orders VALUES ( NULL, ?, ?, ?, ? )", ord.Date, ord.Member, ord.Status, ord.Uuid)

	if err != nil {
		return Order{}, err
	} else {
		var id int64
		id, err = res.LastInsertId()

		if err != nil {
			return Order{}, err
		} else {
			ord.Id = id
			return ord, nil
		}
	}
}

func FetchOrder(id int64, database *sql.DB) (Order, error) {
	rows, err := database.Query("SELECT * FROM orders WHERE id = ?", id)

	if err != nil {
		return Order{}, err
	}

	if !rows.Next() {
		return Order{}, errors.New("No such order")
	}

	ord, err := OrderFromRow(rows)

	rows.Close()

	return ord, err
}

type Receipt struct {
	Order Order
	Cart  []CartItem
	Sum   uint64
}

func FetchReceipt(id int64, database *sql.DB) (Receipt, error) {
	ord, err := FetchOrder(id, database)
	if err != nil {
		return Receipt{}, err
	}

	rows, err := database.Query("SELECT products.id,products.name,products.slug,products.description,products.price,products.count,order_items.count as selected_count FROM order_items JOIN products ON products.id = order_items.product WHERE orderid = ?", id)
	if err != nil {
		return Receipt{}, err
	}

	var sum uint64
	sum = 0
	cart := make([]CartItem, 0)
	for rows.Next() {
		var name, slug, desc string
		var id int64
		var price, count, amount uint64

		err = rows.Scan(&id, &name, &slug, &desc, &price, &count, &amount)
		prod := Product{Id: id, Name: name, Slug: slug, Description: desc, Price: price, Count: count}
		itm := CartItem{Product: prod, Amount: amount, NextAmount: amount + 1, PrevAmount: amount - 1}

		if err != nil {
			rows.Close()
			return Receipt{}, err
		}

		sum += prod.Price * itm.Amount
		cart = append(cart, itm)
	}

	rows.Close()
	return Receipt{ord, cart, sum}, nil
}

func PostNewOrder(session Session, member Member, w http.ResponseWriter, r *http.Request) {
	if member.Id == 0 {
		http.Error(w, "Please Login/Register first", 500)
		return
	}

	uu := uuid.NewRandom()

	DatabaseMutex.Lock()
	var rows *sql.Rows

	tx, err := Database.Begin()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to order: "+err.Error(), 500)
		return
	}

	ord, err := NewOrder(member, uu.String(), tx)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to order: "+err.Error(), 500)
		return
	}

	rows, err = tx.Query("SELECT products.id,products.name,products.slug,products.description,products.price,products.count,carts.count as selected_count FROM carts JOIN products ON products.id = carts.product WHERE session = ?", session.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to order: "+err.Error(), 500)
		return
	}

	var sum uint64
	sum = 0
	cart := make([]CartItem, 0)
	for rows.Next() {
		var name, slug, desc string
		var id int64
		var price, count, amount uint64

		err = rows.Scan(&id, &name, &slug, &desc, &price, &count, &amount)
		prod := Product{Id: id, Name: name, Slug: slug, Description: desc, Price: price, Count: count}
		itm := CartItem{Product: prod, Amount: amount, NextAmount: amount + 1, PrevAmount: amount - 1}

		if err != nil {
			rows.Close()
			tx.Rollback()
			DatabaseMutex.Unlock()
			http.Error(w, "Failed to order: "+err.Error(), 500)
			return
		}

		sum += prod.Price * itm.Amount
		cart = append(cart, itm)
	}

	for _, c := range cart {
		_, err = tx.Exec("INSERT INTO order_items VALUES ( ?, ?, ? )", ord.Id, c.Product.Id, c.Amount)
		if err != nil {
			tx.Rollback()
			DatabaseMutex.Unlock()
			http.Error(w, "Failed to order: "+err.Error(), 500)
			return
		}
	}

	if len(cart) == 0 {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to order: empty cart", 500)
		return
	}

	_, err = tx.Exec("DELETE FROM carts WHERE session = ?", session.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to order: "+err.Error(), 500)
		return
	}

	err = tx.Commit()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed add to cart: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Unlock()

	meta := struct {
		Uuid uuid.UUID
		Sum  uint64
	}{
		uu,
		sum,
	}

	RenderTemplate(w, "orders/success", "", member, meta)
}

func GetNewOrder(session Session, member Member, w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()

	var rows *sql.Rows
	rows, err := Database.Query("SELECT products.id,products.name,products.slug,products.description,products.price,products.count,carts.count as selected_count FROM carts JOIN products ON products.id = carts.product WHERE session = ?", session.Id)

	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed fetch cart: "+err.Error(), 500)
		return
	}

	var sum uint64
	sum = 0
	cart := make([]CartItem, 0)
	for rows.Next() {
		var name, slug, desc string
		var id int64
		var price, count, amount uint64

		err := rows.Scan(&id, &name, &slug, &desc, &price, &count, &amount)
		prod := Product{Id: id, Name: name, Slug: slug, Description: desc, Price: price, Count: count}
		itm := CartItem{Product: prod, Amount: amount, NextAmount: amount + 1, PrevAmount: amount - 1}

		if err == nil {
			sum += prod.Price * itm.Amount
			cart = append(cart, itm)
		}
	}

	DatabaseMutex.Unlock()

	meta := struct {
		Cart []CartItem
		Sum  uint64
	}{
		cart,
		sum,
	}

	RenderTemplate(w, "orders/new", "", member, meta)

}

func HandleOrdersNew(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		return
	}

	fmt.Println("HandleOrderNew() Path = '" + r.URL.Path + "', Method = " + r.Method)

	if r.Method == "POST" {
		PostNewOrder(sess, mem, w, r)
	} else if r.Method == "GET" {
		GetNewOrder(sess, mem, w, r)
	} else {
		http.Error(w, "Method not supported", 405)
	}
}

type NamedReceipt struct {
	Receipt Receipt
	Member  Member
}

func GetOrders(mem Member, w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	rows, err := Database.Query("SELECT id,member FROM orders")

	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}

	rcpts := make([]NamedReceipt, 0)
	for rows.Next() {
		var id, memId int64

		err := rows.Scan(&id, &memId)
		if err != nil {
			rows.Close()
			DatabaseMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}

		rcpt, err := FetchReceipt(id, Database)

		if err != nil {
			rows.Close()
			DatabaseMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}

		rec_mem, err := FetchMember(memId, Database)

		if err != nil {
			rows.Close()
			DatabaseMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}

		rcpts = append(rcpts, NamedReceipt{Receipt: rcpt, Member: rec_mem})
	}

	DatabaseMutex.Unlock()
	RenderTemplate(w, "orders/list", "", mem, rcpts)
}

func PutOrder(rcpt Receipt, w http.ResponseWriter, r *http.Request) {
	stats, ok := r.PostForm["status"]
	if !ok || len(stats) != 1 || (stats[0] != "new" && stats[0] != "paid") {
		http.Error(w, "Missing or invalid status", 500)
		return
	}

	DatabaseMutex.Lock()
	_, err := Database.Exec("UPDATE orders SET status = ? WHERE id = ?", stats[0], rcpt.Order.Id)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to update order: "+err.Error(), 500)
	} else {
		http.Redirect(w, r, "/orders/", 301)
	}
}

func DeleteOrder(rcpt Receipt, w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	tx, err := Database.Begin()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to delete order: "+err.Error(), 500)
		return
	}

	for _, itm := range rcpt.Cart {
		rows, err := tx.Query("SELECT count FROM products WHERE id = ?", itm.Product.Id)
		if err != nil {
			tx.Rollback()
			DatabaseMutex.Unlock()
			http.Error(w, "Failed to delete order: "+err.Error(), 500)
			return
		}

		if !rows.Next() {
			tx.Rollback()
			DatabaseMutex.Unlock()
			http.Error(w, "Failed to delete order: ordered non-existing product", 500)
			return
		}

		var count uint64
		err = rows.Scan(&count)
		if err != nil {
			tx.Rollback()
			DatabaseMutex.Unlock()
			http.Error(w, "Failed to delete order: "+err.Error(), 500)
			return
		}

		rows.Close()
		count += itm.Amount
		_, err = tx.Exec("UPDATE products SET count = ? WHERE id = ?", count, itm.Product.Id)
		if err != nil {
			tx.Rollback()
			DatabaseMutex.Unlock()
			http.Error(w, "Failed to delete order: "+err.Error(), 500)
			return
		}
	}

	_, err = tx.Exec("DELETE FROM order_items WHERE orderid = ?", rcpt.Order.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to delete order: "+err.Error(), 500)
		return
	}

	_, err = tx.Exec("DELETE FROM orders WHERE id = ?", rcpt.Order.Id)
	if err != nil {
		tx.Rollback()
		DatabaseMutex.Unlock()
		http.Error(w, "Failed to delete order: "+err.Error(), 500)
		return
	}

	err = tx.Commit()
	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, "Failed delete order: "+err.Error(), 500)
		return
	}

	DatabaseMutex.Unlock()
	http.Redirect(w, r, "/orders/", 301)
}

func HandleOrder(w http.ResponseWriter, r *http.Request) {
	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)
	DatabaseMutex.Unlock()

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		return
	}

	if mem.Group != "admin" {
		http.Error(w, "Insufficient permissions", 403)
		return
	}

	fmt.Println("HandleOrder() Path = '" + r.URL.Path + "', Method = " + r.Method)

	if r.URL.Path == "/orders" || r.URL.Path == "/orders/" {
		if r.Method == "GET" {
			GetOrders(mem, w, r)
		} else {
			http.Error(w, "Method not supported", 405)
		}
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

		ordId, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/orders/"), 10, 64)

		if err != nil {
			http.Error(w, "Order not found: "+err.Error(), 404)
			return
		}

		DatabaseMutex.Lock()
		rcpt, err := FetchReceipt(ordId, Database)
		DatabaseMutex.Unlock()

		if err != nil {
			http.Error(w, "Order not found: "+err.Error(), 404)
			return
		}

		if meth == "PUT" {
			PutOrder(rcpt, w, r)
		} else if meth == "DELETE" {
			DeleteOrder(rcpt, w, r)
		} else {
			http.Error(w, "Not found", 404)
		}
	} else {
		http.Error(w, "Method not supported", 405)
	}
}

func GetMyOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not supported", 405)
		return
	}

	DatabaseMutex.Lock()
	sess := FetchOrCreateSession(w, r, Database)
	mem, err := FetchMember(sess.Member, Database)

	if err != nil {
		http.Error(w, "Failed to fetch member: "+err.Error(), 500)
		DatabaseMutex.Unlock()
		return
	}

	if mem.Id == 0 {
		http.Error(w, "Please login first", 500)
		DatabaseMutex.Unlock()
		return
	}

	rows, err := Database.Query("SELECT id FROM orders WHERE member = ?", mem.Id)

	if err != nil {
		DatabaseMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}

	orders := make([]Receipt, 0)
	for rows.Next() {
		var id int64

		err := rows.Scan(&id)
		if err != nil {
			rows.Close()
			DatabaseMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}

		ord, err := FetchReceipt(id, Database)

		if err != nil {
			rows.Close()
			DatabaseMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}

		orders = append(orders, ord)
	}

	DatabaseMutex.Unlock()

	RenderTemplate(w, "orders/my", "", mem, orders)
}
