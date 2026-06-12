package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/francisguimond/kairos/client"
)

func ExampleClient() {
	ctx := context.Background()
	c := client.New("http://localhost:8080")

	s, err := c.Create(ctx, client.CreateScheduleInput{
		Name:     "nightly-report",
		CronExpr: "0 0 * * *",
		Payload:  json.RawMessage(`{"job":"report"}`),
	})
	if err != nil {
		log.Fatal(err)
	}

	if _, err := c.Pause(ctx, s.ID); err != nil {
		log.Fatal(err)
	}

	fmt.Println(s.Name)
}
