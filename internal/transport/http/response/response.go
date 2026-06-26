package response

import (
	"net/http"

	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    any          `json:"data,omitempty"`
	Errors  []FieldError `json:"errors,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Param   string `json:"param"`
	Message string `json:"message"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: 0, Message: "ok", Data: data})
}

func FromError(c *gin.Context, err error) {
	status, code, message := xerr.ToHTTP(err)
	c.JSON(status, Envelope{Code: code, Message: message, Errors: validationFieldErrors(err)})
}

func BadRequest(c *gin.Context, err error) {
	FromError(c, xerr.Validation(err))
}
