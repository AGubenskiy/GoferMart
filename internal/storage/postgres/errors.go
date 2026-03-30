package postgres

import "errors"

var (
	ErrDuplicateLogin               = errors.New("login already exists")
	ErrUserNotFound                 = errors.New("user not found")
	ErrOrderNotFound                = errors.New("order not found")
	ErrOrderAlreadyUploadedByUser   = errors.New("order already uploaded by user")
	ErrOrderUploadedByAnotherUser   = errors.New("order uploaded by another user")
	ErrWithdrawalOrderAlreadyExists = errors.New("withdrawal order already exists")
)
