package pyroql

import "errors"

var (
	ErrInvalidAppName     = errors.New("invalid application name")
	ErrAppNameIsRequired  = errors.New("application name is required")
	ErrInvalidQuerySyntax = errors.New("invalid query syntax")

	ErrUnknownOp               = errors.New("unknown tag match operator")
	ErrMatchOperatorIsRequired = errors.New("tag match operator is required")
	ErrInvalidValueSyntax      = errors.New("invalid tag value syntax")
	ErrInvalidMatchersSyntax   = errors.New("invalid tag matchers syntax")
	ErrInvalidKey              = errors.New("invalid tag key")
	ErrKeyReserved             = errors.New("tag key is reserved")
)

type Error struct {
	Inner error
	Expr  string
	// TODO: add offset
}

func newErr(err error, expr string) *Error { return &Error{Inner: err, Expr: expr} }

func (e *Error) Error() string { return e.Inner.Error() + ": " + e.Expr }

func (e *Error) Unwrap() error { return e.Inner }
