package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

type AlertDirection string

const (
	DirectionAbove AlertDirection = "ABOVE"
	DirectionBelow AlertDirection = "BELOW"
)

type PriceAlert struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	Symbol      string          `json:"symbol"`
	TargetPrice decimal.Decimal `json:"target_price"`
	Direction   AlertDirection  `json:"direction"`
	IsActive    bool            `json:"is_active"`
}

type NotificationTask struct {
	AlertID     string          `json:"alert_id"`
	UserID      string          `json:"user_id"`
	Symbol      string          `json:"symbol"`
	TargetPrice decimal.Decimal `json:"target_price"`
	ActualPrice decimal.Decimal `json:"actual_price"`
	Direction   AlertDirection  `json:"direction"`
}

type AlertEngine struct {
	rdb     *redis.Client
	mu      sync.RWMutex
	alerts  map[string][]*PriceAlert // Keyed by Symbol
}

func NewAlertEngine(rdb *redis.Client) *AlertEngine {
	return &AlertEngine{
		rdb:    rdb,
		alerts: make(map[string][]*PriceAlert),
	}
}

// AddAlert saves a new alert in memory (in production this would also save to Postgres)
func (e *AlertEngine) AddAlert(userID, symbol string, targetPrice decimal.Decimal, direction AlertDirection) *PriceAlert {
	alert := &PriceAlert{
		ID:          uuid.New().String(),
		UserID:      userID,
		Symbol:      symbol,
		TargetPrice: targetPrice,
		Direction:   direction,
		IsActive:    true,
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.alerts[symbol] = append(e.alerts[symbol], alert)
	return alert
}

// CheckPrice is called whenever a new trade occurs
func (e *AlertEngine) CheckPrice(symbol string, currentPrice decimal.Decimal) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbolAlerts := e.alerts[symbol]
	if len(symbolAlerts) == 0 {
		return
	}

	var activeAlerts []*PriceAlert

	for _, alert := range symbolAlerts {
		if !alert.IsActive {
			continue
		}

		triggered := false
		if alert.Direction == DirectionAbove && currentPrice.GreaterThanOrEqual(alert.TargetPrice) {
			triggered = true
		} else if alert.Direction == DirectionBelow && currentPrice.LessThanOrEqual(alert.TargetPrice) {
			triggered = true
		}

		if triggered {
			alert.IsActive = false
			log.Printf("[AlertEngine] Alert Triggered for %s! Target: %s, Current: %s", alert.UserID, alert.TargetPrice, currentPrice)
			
			// Push to Redis Streams
			task := NotificationTask{
				AlertID:     alert.ID,
				UserID:      alert.UserID,
				Symbol:      alert.Symbol,
				TargetPrice: alert.TargetPrice,
				ActualPrice: currentPrice,
				Direction:   alert.Direction,
			}
			taskJSON, _ := json.Marshal(task)

			e.rdb.XAdd(context.Background(), &redis.XAddArgs{
				Stream: "notifications:queue",
				Values: map[string]interface{}{
					"payload": string(taskJSON),
				},
			})
		} else {
			// keep in active list
			activeAlerts = append(activeAlerts, alert)
		}
	}

	// Update the symbol's alert list to only contain non-triggered alerts
	e.alerts[symbol] = activeAlerts
}

func StartNotificationWorker(ctx context.Context, rdb *redis.Client) {
	log.Println("Starting NotificationWorker...")

	// Create Consumer Group (Ignore error if it already exists)
	rdb.XGroupCreateMkStream(ctx, "notifications:queue", "notify_group", "0")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Read from stream
				streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    "notify_group",
					Consumer: "worker-1",
					Streams:  []string{"notifications:queue", ">"},
					Count:    10,
					Block:    2 * time.Second,
				}).Result()

				if err != nil && err != redis.Nil {
					log.Printf("Error reading from Redis Stream: %v", err)
					time.Sleep(1 * time.Second)
					continue
				}

				if len(streams) == 0 {
					continue
				}

				for _, stream := range streams {
					for _, msg := range stream.Messages {
						payloadStr := msg.Values["payload"].(string)
						var task NotificationTask
						if err := json.Unmarshal([]byte(payloadStr), &task); err != nil {
							log.Printf("Failed to unmarshal notification task: %v", err)
							continue
						}

						// Process Notification
						sendTelegramNotification(task)
						sendWebPushNotification(task)

						// Acknowledge message
						rdb.XAck(ctx, "notifications:queue", "notify_group", msg.ID)
					}
				}
			}
		}
	}()
}

func sendTelegramNotification(task NotificationTask) {
	// In reality this would be an HTTP POST to https://api.telegram.org/bot<token>/sendMessage
	msg := fmt.Sprintf("🚨 PRICE ALERT! %s crossed %s %s. Actual Price: %s", task.Symbol, task.Direction, task.TargetPrice, task.ActualPrice)
	log.Printf("[TelegramBot] Sending to User %s: %s", task.UserID, msg)
}

func sendWebPushNotification(task NotificationTask) {
	// In reality this would be an HTTP POST using a WebPush library (VAPID)
	msg := fmt.Sprintf("🚨 %s is now %s", task.Symbol, task.ActualPrice)
	log.Printf("[WebPush] Sending to browser of User %s: %s", task.UserID, msg)
}
