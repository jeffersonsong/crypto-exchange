package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jeffersonsong/crypto-exchange/client"
	"github.com/jeffersonsong/crypto-exchange/server"
)

const (
	maxOrders = 3
)

var (
	tick = 2 * time.Second
)

func marketOrderPlacer(c *client.Client) {
	ticker := time.NewTicker(5 * time.Second)

	for {
		marketSellOrder := &client.PlaceOrderParams{
			UserID: 666,
			Bid:    false,
			Size:   1000,
		}

		orderResp, err := c.PlaceMarketOrder(marketSellOrder)
		if err != nil {
			log.Println(orderResp.OrderID)
		}

		marketBuyOrder := &client.PlaceOrderParams{
			UserID: 666,
			Bid:    true,
			Size:   1000,
		}

		orderResp, err = c.PlaceMarketOrder(marketBuyOrder)
		if err != nil {
			log.Println(orderResp.OrderID)
		}

		<-ticker.C
	}
}

func makeMarketSimple(c *client.Client) {
	ticker := time.NewTicker(tick)

	for {
		orders, err := c.GetOrders(7)
		if err != nil {
			log.Println(err)
		}

		bestAsk, err := c.GetBestAsk()
		if err != nil {
			log.Println(err)
		}
		bestBid, err := c.GetBestBid()
		if err != nil {
			log.Println(err)
		}

		spread := math.Abs(bestAsk - bestBid)
		fmt.Printf("best bid: %.2f, ask: %.2f, spread: %.2f\n", bestBid, bestAsk, spread)

		// place the bid
		if len(orders.Bids) < maxOrders {
			bidLimit := &client.PlaceOrderParams{
				UserID: 7,
				Bid:    true,
				Price:  bestBid + 100,
				Size:   1000,
			}

			bidOrderResp, err := c.PlaceLimitOrder(bidLimit)
			if err != nil {
				log.Println(bidOrderResp.OrderID)
			}
		}

		// place the ask
		if len(orders.Asks) < maxOrders {
			askLimit := &client.PlaceOrderParams{
				UserID: 7,
				Bid:    false,
				Price:  bestAsk - 100,
				Size:   1000,
			}

			askOrderResp, err := c.PlaceLimitOrder(askLimit)
			if err != nil {
				log.Println(askOrderResp.OrderID)
			}
		}

		<-ticker.C
	}
}

func seedMarket(c *client.Client) error {
	ask := &client.PlaceOrderParams{
		UserID: 8,
		Bid:    false,
		Price:  10_000.0,
		Size:   1_000_000.0,
	}

	bid := &client.PlaceOrderParams{
		UserID: 8,
		Bid:    true,
		Price:  9_000.0,
		Size:   1_000_000.0,
	}

	_, err := c.PlaceLimitOrder(ask)
	if err != nil {
		return err
	}

	_, err = c.PlaceLimitOrder(bid)
	if err != nil {
		return err
	}

	return nil
}

// TODO - Migrate to Python
func main() {
	go server.StartServer()

	time.Sleep(1 * time.Second)

	c := client.NewClient()

	if err := seedMarket(c); err != nil {
		panic(err)
	}

	go makeMarketSimple(c)

	time.Sleep(1 * time.Second)

	marketOrderPlacer(c)

	select {}
}
