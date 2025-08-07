package main

import (
	"context"
	"fmt"

	pg "github.com/eliona-smart-building-assistant/backend-frm/pkg/postgres"
)

func main() {
	dbPool, err := pg.NewPool(context.Background(),
		pg.WithHostname("localhost"),
		pg.WithPort(5432),
		pg.WithCredentials("postgres", "h2WGqWQ4"),
		pg.WithDatabase("iot"),
	)

	if err != nil {
		panic(err)
	}

	ct, err := dbPool.Query(context.Background(), "SELECT 1")
	fmt.Println(ct, err)
}
