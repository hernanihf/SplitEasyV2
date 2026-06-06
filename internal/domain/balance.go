package domain

type UserBalance struct {
	UserID uint
	Amount float64 // Positive means they should receive money, Negative means they owe money
}

type Debt struct {
	FromUserID uint    `json:"from_user_id"`
	ToUserID   uint    `json:"to_user_id"`
	Amount     float64 `json:"amount"`
}
