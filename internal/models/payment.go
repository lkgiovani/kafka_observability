package models

import "time"

type Payment struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	TotalCents  int64     `json:"total_cents"`
	Status      string    `json:"status"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}
