package domain

type UserBalance struct {
	UserID uint
	Amount int64 // cents; positive means they should receive money, negative means they owe
}

type Debt struct {
	FromUserID uint  `json:"from_user_id"`
	ToUserID   uint  `json:"to_user_id"`
	Amount     int64 `json:"amount"` // cents
}
