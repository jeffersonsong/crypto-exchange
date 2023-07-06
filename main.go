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
		UserID    int64
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

	UserData struct {
		ID         int64
		PrivateKey string
	}
)

func main() {
	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler

	client, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		log.Fatal(err)
	}

	ex, err := NewExchange(exchangePrivateKey, client)
	if err != nil {
		log.Fatal(err)
	}

	userDataList := []UserData{
		{ID: 8, PrivateKey: "829e924fdf021ba3dbbc4225edfece9aca04b929d6e75613329ca6f1d31c0bb4"},
		{ID: 7, PrivateKey: "a453611d9419d0e56f499079478fd72c37b251a94bfde4d19872c44cf65386e3"},
		{ID: 666, PrivateKey: "e485d098507f54e7733a205420dfddbe58db035fa577fc294ebd14db90767a52"},
	}

	for _, userData := range userDataList {
		ex.AddUser(userData)
	}

	e.GET("/book/:market", ex.handleGetBook)
	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/order/:id", ex.cancelOrder)
	e.GET("/balance/:userID", ex.handleGetBalance)
	e.GET("/balances", ex.handleGetBalances)

	address := "0xACa94ef8bD5ffEE41947b4585a84BdA5a3d3DA6E"
	balance, _ := ex.Client.BalanceAt(context.Background(), common.HexToAddress(address), nil)
	fmt.Println(balance)

	e.Start(":3000")
}

type User struct {
	ID         int64
	PrivateKey *ecdsa.PrivateKey
}

func NewUser(privateKey string, id int64) *User {
	pk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}

	return &User{
		ID:         id,
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
		UserID:    order.UserID,
		ID:        order.ID,
		Price:     price,
		Size:      order.Size,
		Bid:       order.Bid,
		Timestamp: order.Timestamp,
	}
}

func (ex *Exchange) AddUser(userData UserData) (*User, error) {
	_, ok := ex.Users[userData.ID]
	if ok {
		return nil, fmt.Errorf("USer %d already exists", userData.ID)
	}
	user := NewUser(userData.PrivateKey, userData.ID)
	ex.Users[user.ID] = user
	return user, nil
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

	return nil
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
	for _, match := range matches {
		fromUser, ok := ex.Users[match.Ask.UserID]
		if !ok {
			return fmt.Errorf("User not found: %d", match.Ask.UserID)
		}

		toUser, ok := ex.Users[match.Bid.UserID]
		if !ok {
			return fmt.Errorf("User not found: %d", match.Bid.UserID)
		}
		toAddress := crypto.PubkeyToAddress(toUser.PrivateKey.PublicKey)

		// this is only used for the fees
		// exchangePubKey := ex.PrivateKey.Public()
		// publicKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)
		// if !ok {
		// 	return fmt.Errorf("error casting public key to ECDSA")
		// }
		//toAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
		amount := big.NewInt(int64(match.SizeFilled))

		err := transferETH(ex.Client, fromUser.PrivateKey, toAddress, amount)
		if err != nil {
			log.Fatal(err)
		}
		// TODO - how to handle error
	}
	return nil
}

func (ex *Exchange) balance(userID int64) (*big.Int, error) {
	user, ok := ex.Users[userID]
	if !ok {
		return nil, fmt.Errorf("User not found: %d", userID)
	}
	address := crypto.PubkeyToAddress(user.PrivateKey.PublicKey)
	return ex.Client.BalanceAt(context.Background(), address, nil)
}

func (ex *Exchange) handleGetBalance(c echo.Context) error {
	userIDStr := c.Param("userID")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return err
	}

	balance, err := ex.balance(int64(userID))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{"balance": balance})
}

func (ex *Exchange) handleGetBalances(c echo.Context) error {
	balances := make(map[int64]*big.Int)
	for _, user := range ex.Users {
		balance, err := ex.balance(user.ID)
		if err != nil {
			return err
		}
		balances[user.ID] = balance
	}

	return c.JSON(http.StatusOK, map[string]any{"balances": balances})
}
