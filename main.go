package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jeffersonsong/crypto-exchange/orderbook"
	"github.com/labstack/echo/v4"
)

const (
	MarketETH Market = "ETH"

	MarketOrder OrderType = "MARKET"
	LimitOrder  OrderType = "LIMIT"

	exchangePrivateKey = "4f3edf983ac636a65a842ce7c78d9aa706d3b113bce9c46f30d7d21715b23b1d"
)

type (
	OrderType string
	Market    string

	PlaceOrderRequest struct {
		UserID int64
		Type   OrderType // limit or market
		Bid    bool
		Size   float64
		Price  float64
		Market Market
	}

	Order struct {
		ID        int64
		Price     float64
		Size      float64
		Bid       bool
		Timestamp int64
	}

	OrderbookData struct {
		TotalBidVolume float64
		TotalAskVolume float64
		Asks           []*Order
		Bids           []*Order
	}

	MatchedOrder struct {
		Price float64
		Size  float64
		ID    int64
	}
)

func main() {
	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler

	client, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Dial")

	ex, err := NewExchange(exchangePrivateKey, client)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("NewExchange")

	pkStr := "829e924fdf021ba3dbbc4225edfece9aca04b929d6e75613329ca6f1d31c0bb4"
	pk, err := crypto.HexToECDSA(pkStr)
	if err != nil {
		panic(err)
	}
	fmt.Println("HexToECDSA")

	user := &User{
		ID:         8,
		PrivateKey: pk,
	}

	ex.Users[user.ID] = user

	e.GET("/book/:market", ex.handleGetBook)
	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/order/:id", ex.cancelOrder)

	address := "0xACa94ef8bD5ffEE41947b4585a84BdA5a3d3DA6E"
	balance, _ := ex.Client.BalanceAt(context.Background(), common.HexToAddress(address), nil)
	fmt.Println(balance)

	// privateKey, err := crypto.HexToECDSA(exchangePrivateKey)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// publicKey := privateKey.Public()
	// publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	// if !ok {
	// 	log.Fatal("error casting public key to ECDSA")
	// }

	// fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	// nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// value := big.NewInt(1000000000000000000) // in wei (1 eth)
	// gasLimit := uint64(21000)                // in units

	// gasPrice, err := client.SuggestGasPrice(context.Background())
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// toAddress := common.HexToAddress("0x1dF62f291b2E969fB0849d99D9Ce41e2F137006e")
	// tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, nil)

	// // chainID, err := client.NetworkID(context.Background())
	// // if err != nil {
	// // 	log.Fatal(err)
	// // }
	// // fmt.Printf("Chain id: %+v\n", chainID)

	// chainID := big.NewInt(1337)

	// signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// err = client.SendTransaction(context.Background(), signedTx)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// balance, err := client.BalanceAt(ctx, toAddress, nil)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// fmt.Printf("balance: %v\n", balance)

	e.Start(":3000")
}

type User struct {
	ID         int64
	PrivateKey *ecdsa.PrivateKey
}

func NewUser(privateKey string) *User {
	pk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}
	return &User{
		PrivateKey: pk,
	}
}

func httpErrorHandler(err error, c echo.Context) {
	fmt.Println(err)
}

type Exchange struct {
	Client     *ethclient.Client
	Users      map[int64]*User
	orders     map[int64]int64
	PrivateKey *ecdsa.PrivateKey
	orderbooks map[Market]*orderbook.Orderbook
}

func NewExchange(privateKey string, client *ethclient.Client) (*Exchange, error) {
	orderbooks := make(map[Market]*orderbook.Orderbook)
	orderbooks[MarketETH] = orderbook.NewOrderbook()

	pk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}
	return &Exchange{
		Client:     client,
		Users:      make(map[int64]*User),
		orders:     make(map[int64]int64),
		PrivateKey: pk,
		orderbooks: orderbooks,
	}, nil
}

func NewOrder(price float64, order *orderbook.Order) *Order {
	return &Order{
		ID:        order.ID,
		Price:     price,
		Size:      order.Size,
		Bid:       order.Bid,
		Timestamp: order.Timestamp,
	}
}

func (ex *Exchange) handleGetBook(c echo.Context) error {
	market := Market(c.Param("market"))

	ob, ok := ex.orderbooks[market]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"msg": "market not found"})
	}

	orderbookData := OrderbookData{
		TotalBidVolume: ob.BidTotalVolume(),
		TotalAskVolume: ob.AskTotalVolume(),
		Asks:           []*Order{},
		Bids:           []*Order{},
	}

	for _, limit := range ob.Asks() {
		for _, order := range limit.Orders {
			o := NewOrder(limit.Price, order)
			orderbookData.Asks = append(orderbookData.Asks, o)
		}
	}

	for _, limit := range ob.Bids() {
		for _, order := range limit.Orders {
			o := NewOrder(limit.Price, order)
			orderbookData.Bids = append(orderbookData.Bids, o)
		}
	}

	return c.JSON(http.StatusOK, orderbookData)
}

func (ex *Exchange) cancelOrder(c echo.Context) error {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	ob := ex.orderbooks[MarketETH]
	order, ok := ob.Orders[int64(id)]

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"msg": "Order not found"})
	}
	ob.CancelOrder(order)
	return c.JSON(http.StatusOK, map[string]any{"msg": "Order deleted"})
}

func (ex *Exchange) handlePlaceMarketOrder(market Market, order *orderbook.Order) ([]orderbook.Match, []*MatchedOrder) {
	ob := ex.orderbooks[market]
	matches := ob.PlaceMarketOrder(order)
	matchedOrders := make([]*MatchedOrder, len(matches))

	for i, match := range matches {
		var id int64
		if match.Ask.Bid != order.Bid {
			id = match.Ask.ID
		} else {
			id = match.Bid.ID
		}
		matchedOrders[i] = &MatchedOrder{
			ID:    id,
			Size:  match.SizeFilled,
			Price: match.Price,
		}
	}

	return matches, matchedOrders
}

func (ex *Exchange) handlePlaceLimitOrder(market Market, price float64, order *orderbook.Order) error {
	ob := ex.orderbooks[market]
	ob.PlaceLimitOrder(price, order)

	// if order.Bid {
	// 	return nil
	// }

	user, ok := ex.Users[order.UserID]
	if !ok {
		return fmt.Errorf("User not found: %d", order.UserID)
	}

	exchangePubKey := ex.PrivateKey.Public()
	publicKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}
	toAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	amount := big.NewInt(int64(order.Size))

	return transferETH(ex.Client, user.PrivateKey, toAddress, amount)
}

func (ex *Exchange) handlePlaceOrder(c echo.Context) error {
	var placeOrderData PlaceOrderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&placeOrderData); err != nil {
		return err
	}

	order := orderbook.NewOrder(placeOrderData.Bid, placeOrderData.Size, placeOrderData.UserID)

	if placeOrderData.Type == LimitOrder {
		if err := ex.handlePlaceLimitOrder(placeOrderData.Market, placeOrderData.Price, order); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{"msg": "limit order placed"})

	} else if placeOrderData.Type == MarketOrder {
		matches, matchedOrders := ex.handlePlaceMarketOrder(placeOrderData.Market, order)

		if err := ex.handleMatches(matches); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{"matches": matchedOrders})

	} else {
		return c.JSON(http.StatusBadRequest, map[string]any{"msg": "invalid order type"})
	}
}

func (ex *Exchange) handleMatches(matches []orderbook.Match) error {
	return nil
}
