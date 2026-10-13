package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
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
	log.Println("Starting Matchbox Load Test...")

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	
	authUrl := "http://localhost:8080/api/v1/auth/mock-login?user_id=loadtest-user"
	authReq, _ := http.NewRequest("POST", authUrl, nil)
	authResp, authErr := client.Do(authReq)
	if authErr != nil {
		log.Fatalf("Mock login failed: %v", authErr)
	}
	
	authBodyBytes := new(bytes.Buffer)
	authBodyBytes.ReadFrom(authResp.Body)
	authResp.Body.Close()
	
	log.Printf("Auth status: %d, body: %s", authResp.StatusCode, authBodyBytes.String())
	
	var authData struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(authBodyBytes.Bytes(), &authData); err != nil {
		log.Fatalf("Failed to decode auth response: %v", err)
	}
	
	if authData.AccessToken == "" {
		log.Fatalf("Failed to retrieve access token. Body was: %s", authBodyBytes.String())
	}
	authToken := authData.AccessToken
	log.Println("Successfully acquired auth token")

	url := "http://localhost:8080/api/v1/order"

	numWorkers := 50
	ordersPerWorker := 100

	var wg sync.WaitGroup
	startTime := time.Now()
	var successCount, errorCount int
	var mu sync.Mutex

	basePrice := 65000.0

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for j := 0; j < ordersPerWorker; j++ {
				// Randomize order
				side := "BUY"
				if r.Intn(2) == 0 {
					side = "SELL"
				}

				// Price around basePrice +/- $1000
				priceDiff := (r.Float64() * 2000) - 1000
				price := fmt.Sprintf("%.2f", basePrice+priceDiff)

				// Quantity between 0.01 and 1.5
				qty := fmt.Sprintf("%.4f", 0.01+(r.Float64()*1.49))

				// Make it interesting: occasionally a whale order
				if r.Intn(100) < 5 {
					qty = fmt.Sprintf("%.2f", 2.0+(r.Float64()*3.0)) // 2 to 5 BTC
				}

				reqBody := OrderRequest{
					Symbol:   "BTC/USDT",
					Side:     side,
					Type:     "LIMIT",
					Price:    price,
					Quantity: qty,
				}

				jsonData, _ := json.Marshal(reqBody)
				req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+authToken)

				resp, err := client.Do(req)
				if err != nil {
					mu.Lock()
					errorCount++
					mu.Unlock()
					continue
				}
				
				if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
					mu.Lock()
					successCount++
					mu.Unlock()
				} else {
					mu.Lock()
					if errorCount == 0 { // Print first error for debugging
						buf := new(bytes.Buffer)
						buf.ReadFrom(resp.Body)
						log.Printf("First error response: %d - %s", resp.StatusCode, buf.String())
					}
					errorCount++
					mu.Unlock()
				}
				resp.Body.Close()

				// Small random delay to spread requests
				time.Sleep(time.Duration(r.Intn(50)) * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)
	totalOrders := successCount + errorCount

	log.Println("======================================")
	log.Printf("Load Test Completed in %v\n", duration)
	log.Printf("Total Orders Sent: %d\n", totalOrders)
	log.Printf("Successful Orders: %d\n", successCount)
	log.Printf("Failed Orders:     %d\n", errorCount)
	log.Printf("Throughput:        %.2f orders/sec\n", float64(totalOrders)/duration.Seconds())
	log.Println("======================================")
}
