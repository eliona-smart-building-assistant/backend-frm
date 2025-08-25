package main

import (
	"context"
	"fmt"

	pg "github.com/eliona-smart-building-assistant/backend-frm/pkg/postgres"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/utils"
)

func main() {
	dbLogin := utils.EnvOrDefault("DB_LOGIN", "")
	dbPassword := utils.EnvOrDefault("DB_PASSWORD", "")

	dbPool, err := pg.NewPool(context.Background(),
		pg.WithHostname("localhost"),
		pg.WithPort(5432),
		pg.WithCredentials(dbLogin, dbPassword),
		pg.WithDatabase("iot"),
	)

	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	defer dbPool.Close()

	//rows, err := dbPool.Query(context.Background(), "select 1")
	//fmt.Printf("Q1: %T\n", err)
	//rows.Close()
	//
	//data, err := pg.CollectColumn[int](context.Background(), dbPool, "select * from asset")
	//fmt.Printf("Q2: %s,  %T\n", err, err, data)
	//
	//dbPool.SetCredentials("postgres", "h2WGqWQ41")
	//
	//_, err = dbPool.Query(context.Background(), "SELECT 1")
	//
	//var connErr *pg.ConnectionError
	//if errors.As(err, &connErr) {
	//	fmt.Println("ConnectionError")
	//}
}
