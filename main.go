package main

import (
	"time"

	"github.com/jeffersonsong/crypto-exchange/client"
	"github.com/jeffersonsong/crypto-exchange/server"
)

// TODO - Migrate to Python
func main() {
	go server.StartServer()

	time.Sleep(1 * time.Second)

	c := client.NewClient()

	for {
		limitOrderParams := &client.PlaceOrderParams{
			UserID: 8,
			Bid:    false,
			Price:  10_000.0,
			Size:   500_000.0,
		}

		_, err := c.PlaceLimitOrder(limitOrderParams)
		if err != nil {
			panic(err)
		}

		otherLimitOrderParams := &client.PlaceOrderParams{
			UserID: 666,
			Bid:    false,
			Price:  9_000.0,
			Size:   500_000.0,
		}

		_, err = c.PlaceLimitOrder(otherLimitOrderParams)
		if err != nil {
			panic(err)
		}

		// fmt.Println("placed limit order from the client => ", resp.OrderID)

		marketOrderParams := &client.PlaceOrderParams{
			UserID: 7,
			Bid:    true,
			Size:   1000_000.0,
		}

		_, err = c.PlaceMarketOrder(marketOrderParams)
		if err != nil {
			panic(err)
		}

		// fmt.Println("placed market order from the client => ", resp.OrderID)

		time.Sleep(1 * time.Second)
	}

	select {}
}
