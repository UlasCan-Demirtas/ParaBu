package udf

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"matchbox/internal/repository"
)

// mapResolution converts TradingView UDF resolution to Binance interval string
func mapResolution(res string) string {
	switch res {
	case "1":
		return "1m"
	case "3":
		return "3m"
	case "5":
		return "5m"
	case "15":
		return "15m"
	case "30":
		return "30m"
	case "60":
		return "1h"
	case "120":
		return "2h"
	case "240":
		return "4h"
	case "1D", "D":
		return "1d"
	case "1W", "W":
		return "1w"
	case "1M", "M":
		return "1M"
	default:
		return "1m"
	}
}

func RegisterUDFRoutes(router *gin.RouterGroup, repo *repository.KlineRepo) {
	// /time
	router.GET("/time", func(c *gin.Context) {
		c.String(http.StatusOK, strconv.FormatInt(time.Now().Unix(), 10))
	})

	// /config
	router.GET("/config", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"supported_resolutions":    []string{"1", "5", "15", "30", "60", "240", "1D", "1W", "1M"},
			"supports_group_request":   false,
			"supports_marks":           false,
			"supports_search":          true,
			"supports_timescale_marks": false,
			"supports_time":            true,
		})
	})

	// /symbols
	router.GET("/symbols", func(c *gin.Context) {
		symbol := c.Query("symbol")
		if symbol == "" {
			c.JSON(http.StatusBadRequest, gin.H{"s": "error", "errmsg": "Symbol is required"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name":                  symbol,
			"ticker":                symbol,
			"description":           symbol,
			"type":                  "crypto",
			"session":               "24x7",
			"exchange":              "ParaBu Matchbox",
			"listed_exchange":       "ParaBu Matchbox",
			"timezone":              "Etc/UTC",
			"format":                "price",
			"pricescale":            100, // For 2 decimal places (adjust as needed per symbol)
			"minmov":                1,
			"has_intraday":          true,
			"intraday_multipliers":  []string{"1", "5", "15", "30", "60"},
			"supported_resolutions": []string{"1", "5", "15", "30", "60", "240", "1D", "1W", "1M"},
		})
	})

	// /history
	router.GET("/history", func(c *gin.Context) {
		symbol := c.Query("symbol")
		resolution := c.Query("resolution")
		fromStr := c.Query("from")
		toStr := c.Query("to")

		if symbol == "" || resolution == "" || fromStr == "" || toStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"s": "error", "errmsg": "Missing required parameters"})
			return
		}

		fromUnix, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"s": "error", "errmsg": "Invalid from parameter"})
			return
		}

		toUnix, err := strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"s": "error", "errmsg": "Invalid to parameter"})
			return
		}

		interval := mapResolution(resolution)
		startTime := time.Unix(fromUnix, 0).UTC()
		endTime := time.Unix(toUnix, 0).UTC()

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		klines, err := repo.GetKlines(ctx, symbol, interval, startTime, endTime, 5000)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"s": "error", "errmsg": fmt.Sprintf("DB query failed: %v", err)})
			return
		}

		if len(klines) == 0 {
			c.JSON(http.StatusOK, gin.H{"s": "no_data"})
			return
		}

		// Pre-allocate arrays
		t := make([]int64, len(klines))
		o := make([]float64, len(klines))
		h := make([]float64, len(klines))
		l := make([]float64, len(klines))
		cl := make([]float64, len(klines))
		v := make([]float64, len(klines))

		for i, k := range klines {
			t[i] = k.OpenTime
			o[i] = k.Open
			h[i] = k.High
			l[i] = k.Low
			cl[i] = k.Close
			v[i] = k.Volume
		}

		c.JSON(http.StatusOK, gin.H{
			"s": "ok",
			"t": t,
			"o": o,
			"h": h,
			"l": l,
			"c": cl,
			"v": v,
		})
	})
}
