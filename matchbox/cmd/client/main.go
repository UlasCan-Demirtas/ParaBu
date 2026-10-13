package main

import (
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"
)

type OrderRequest struct {
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	Type     string `json:"type"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

func main() {
	log.Println("Matchbox HFT Simulator Client (REST) starting...")
	time.Sleep(1 * time.Second)

	client := &http.Client{Timeout: 10 * time.Second}
	url := "http://localhost:8080/api/v1/order"

	// 1. Send some Asks (Sell Orders)
	prices := []string{"60000", "60100", "60200", "59900"}
	for i := 0; i < 5; i++ {
		reqBody := OrderRequest{
			Symbol:   "BTC/USDT",
			Side:     "SELL",
			Type:     "LIMIT",
			Price:    prices[rand.Intn(len(prices))],
			Quantity: "1.5",
		}

		jsonData, _ := json.Marshal(reqBody)
		resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Fatalf("could not place order: %v", err)
		}
		resp.Body.Close()
		log.Printf("Placed Sell Order: %s", reqBody.Price)
	}

	time.Sleep(500 * time.Millisecond)
	log.Println("---")

	// 2. Send some Bids (Buy Orders) to trigger matches
	for i := 0; i < 5; i++ {
		reqBody := OrderRequest{
			Symbol:   "BTC/USDT",
			Side:     "BUY",
			Type:     "LIMIT",
			Price:    "60100",
			Quantity: "2.0",
		}

		jsonData, _ := json.Marshal(reqBody)
		resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Fatalf("could not place order: %v", err)
		}
		resp.Body.Close()
		log.Printf("Placed Buy Order at 60100")
	}
}
