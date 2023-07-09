package server

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jeffersonsong/crypto-exchange/orderbook"
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
		UserID int64
		Price  float64
		Size   float64
		ID     int64
	}

	UserData struct {
		ID         int64
		PrivateKey string
	}
)

func StartServer() {
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

	e.POST("/order", ex.handlePlaceOrder)
	e.GET("/order/:userID", ex.handleGetOrders)

	e.GET("/book/:market", ex.handleGetBook)
	e.GET("/book/:market/bid", ex.handleGetBestBid)
	e.GET("/book/:market/ask", ex.handleGetBestAsk)

	e.DELETE("/order/:id", ex.cancelOrder)

	e.GET("/balance/:userID", ex.handleGetBalance)
	e.GET("/balances", ex.handleGetBalances)

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
	Client *ethclient.Client
	Users  map[int64]*User
	// Orders map a user to his orders.
	Orders     map[int64][]*orderbook.Order
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
		Orders:     make(map[int64][]*orderbook.Order),
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

func (ex *Exchange) handleGetOrders(c echo.Context) error {
	userIDStr := c.Param("userID")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return err
	}
	orderbookOrders := ex.Orders[int64(userID)]
	orders := []Order{}

	for _, orderbookOrder := range orderbookOrders {
		// if orderbookOrder.Size == 0 {
		// 	continue
		// }
		order := Order{
			UserID:    orderbookOrder.UserID,
			ID:        orderbookOrder.ID,
			Price:     orderbookOrder.Limit.Price,
			Size:      orderbookOrder.Size,
			Bid:       orderbookOrder.Bid,
			Timestamp: orderbookOrder.Timestamp,
		}
		orders = append(orders, order)
	}

	return c.JSON(http.StatusOK, orders)
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

type PriceResponse struct {
	Price float64
}

func (ex *Exchange) handleGetBestBid(c echo.Context) error {
	market := Market(c.Param("market"))
	ob, ok := ex.orderbooks[market]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"msg": "market not found"})
	}
	if len(ob.Bids()) == 0 {
		return fmt.Errorf("The bids are empty")
	}
	bestBidPrice := ob.Bids()[0].Price

	pr := PriceResponse{
		Price: bestBidPrice,
	}

	return c.JSON(http.StatusOK, pr)
}

func (ex *Exchange) handleGetBestAsk(c echo.Context) error {
	market := Market(c.Param("market"))
	ob, ok := ex.orderbooks[market]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"msg": "market not found"})
	}
	if len(ob.Asks()) == 0 {
		return fmt.Errorf("The asks are empty")
	}
	bestAskPrice := ob.Asks()[0].Price

	pr := PriceResponse{
		Price: bestAskPrice,
	}

	return c.JSON(http.StatusOK, pr)
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

	log.Println("order canceled id => ", id)
	return c.JSON(http.StatusOK, map[string]any{"msg": "Order deleted"})
}

func (ex *Exchange) handlePlaceMarketOrder(market Market, order *orderbook.Order) ([]orderbook.Match, []*MatchedOrder) {
	ob := ex.orderbooks[market]

	matches := ob.PlaceMarketOrder(order)
	matchedOrders := make([]*MatchedOrder, len(matches))

	totalSizeFilled := 0.0
	totalAmount := 0.0
	for i, match := range matches {
		var limitOrder *orderbook.Order
		if match.Ask.Bid != order.Bid {
			limitOrder = match.Ask
		} else {
			limitOrder = match.Bid
		}
		matchedOrders[i] = &MatchedOrder{
			UserID: limitOrder.UserID,
			ID:     limitOrder.ID,
			Size:   match.SizeFilled,
			Price:  match.Price,
		}
		totalSizeFilled += match.SizeFilled
		totalAmount += match.Price * match.SizeFilled
	}

	avgPrice := totalAmount / totalSizeFilled

	log.Printf("filled MARKET order => %d | size [%.2f] | avgPrice [%2f]", order.ID, totalSizeFilled, avgPrice)

	return matches, matchedOrders
}

func (ex *Exchange) handlePlaceLimitOrder(market Market, price float64, order *orderbook.Order) error {
	ob := ex.orderbooks[market]
	ob.PlaceLimitOrder(price, order)

	// keep track of user orders
	ex.Orders[order.UserID] = append(ex.Orders[order.UserID], order)

	log.Printf("new LIMIT order => type: [%t] | price [%.2f] | size [%.2f]", order.Bid, order.Limit.Price, order.Size)

	return nil
}

type PlaceOrderResponse struct {
	OrderID int64
}

func (ex *Exchange) handlePlaceOrder(c echo.Context) error {
	var placeOrderData PlaceOrderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&placeOrderData); err != nil {
		return err
	}

	order := orderbook.NewOrder(placeOrderData.Bid, placeOrderData.Size, placeOrderData.UserID)

	if placeOrderData.Type == LimitOrder { // limit orders
		if err := ex.handlePlaceLimitOrder(placeOrderData.Market, placeOrderData.Price, order); err != nil {
			return err
		}

	} else if placeOrderData.Type == MarketOrder { // market orders
		matches, matchedOrders := ex.handlePlaceMarketOrder(placeOrderData.Market, order)

		if err := ex.handleMatches(matches); err != nil {
			return err
		}
		_ = matchedOrders

		// Delete the users of the user when filled
		for _, matchedOrder := range matchedOrders {
			// if the size if 0 we can delete this order
			userOrders := ex.Orders[matchedOrder.UserID]
			for i := 0; i < len(userOrders); i++ {
				if userOrders[i].IsFilled() && userOrders[i].ID == matchedOrder.ID {
					ex.Orders[matchedOrder.UserID] = DeleteOrderChanged(userOrders, i)
				}
			}
		}
		// return c.JSON(http.StatusOK, map[string]any{"matches": matchedOrders})

		// } else {
		// 	return c.JSON(http.StatusBadRequest, map[string]any{"msg": "invalid order type"})
	}

	resp := &PlaceOrderResponse{OrderID: order.ID}
	return c.JSON(http.StatusOK, resp)
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

func transferETH(client *ethclient.Client, fromPrivKey *ecdsa.PrivateKey, to common.Address, amount *big.Int) error {
	ctx := context.Background()

	publicKey := fromPrivKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return err
	}

	gasLimit := uint64(21000) // in units

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}

	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)

	chainID := big.NewInt(1337)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), fromPrivKey)
	if err != nil {
		return err
	}

	return client.SendTransaction(ctx, signedTx)
}

func DeleteOrderChanged[T any](s []*T, i int) []*T {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
