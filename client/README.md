# Kairos Go client

A standalone, dependency-free Go client for the [Kairos](https://github.com/francisguimond/kairos)
scheduler API. It mirrors every API operation and adds `Pause`/`Resume` helpers.

It is a **separate Go module** (`github.com/francisguimond/kairos/client`) that depends only
on the standard library, so importing it does not pull in the Kairos server's dependencies.

## Install

```bash
go get github.com/francisguimond/kairos/client
```

## Usage

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/francisguimond/kairos/client"
)

func main() {
	ctx := context.Background()

	// WithAPIKey is only needed if the server was started with --api-key.
	c := client.New("http://localhost:8080", client.WithAPIKey("secret"))

	// Create
	s, err := c.Create(ctx, client.CreateScheduleInput{
		Name:     "nightly-report",
		CronExpr: "0 0 * * *",
		Payload:  json.RawMessage(`{"job":"report"}`),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Pause / Resume
	if _, err := c.Pause(ctx, s.ID); err != nil {
		log.Fatal(err)
	}
	resumed, err := c.Resume(ctx, s.ID) // server recalculates next_run_at
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("next run:", resumed.NextRunAt)

	// List with filtering, sorting, and pagination
	res, err := c.List(ctx, client.ListOptions{
		Statuses: []client.Status{client.StatusActive},
		Sort:     []client.SortOrder{{Field: "next_run_at", Dir: client.SortAsc}},
		Limit:    20,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d active schedules\n", res.Total)

	// Get, Update, Delete
	if _, err := c.Get(ctx, s.ID); err != nil {
		if client.IsNotFound(err) {
			fmt.Println("gone")
		}
	}
	if err := c.Delete(ctx, s.ID); err != nil {
		log.Fatal(err)
	}
}
```

## Errors

Non-2xx responses are returned as `*client.APIError` (`StatusCode` + `Message`). Use
`client.IsNotFound(err)` and `client.IsUnauthorized(err)` for common cases, or
`errors.As(err, &apiErr)` for the rest.
