package domain

import "errors"

var (
	ErrPlayerNotFound      = errors.New("player not found")
	ErrAccountNotFound     = errors.New("account not found")
	ErrSessionExpired      = errors.New("session expired")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrCargoFull           = errors.New("cargo inventory is full")
	ErrOutOfRange          = errors.New("target is out of range")
	ErrCooldownActive      = errors.New("weapon/device cooldown active")
	ErrInsufficientCredits = errors.New("insufficient credits")
	ErrInvalidTarget       = errors.New("invalid entity target")
)
