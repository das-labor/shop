GO=go
SOURCES=main.go template.go member.go product.go database.go session.go route.go

.PHONY: run

shop: $(SOURCES)
	$(GO) build -o shop $^

run: shop
	./shop
