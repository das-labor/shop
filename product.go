package main

import (
	"database/sql"
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

func GetProducts() ([]string, error) {
	return []string{"Hello", ", ", "World"}, nil
}
