package domain

import "errors"

// Sentinel errors emitted by the domain layer. These stay internal: the
// service layer maps them to dto.AppError before they cross the handler
// boundary.
var (
	ErrNicknameEmpty   = errors.New("domain: nickname empty")
	ErrNicknameTooLong = errors.New("domain: nickname too long")
	ErrSameName        = errors.New("domain: same name")
)
