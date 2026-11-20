package response

import "github.com/gofiber/fiber/v2"

type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type ErrorBody struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
}

func Success(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(Response{
		Success: true,
		Data:    data,
	})
}

func Error(c *fiber.Ctx, status int, code, message string, details ...ErrorDetail) error {
	return c.Status(status).JSON(Response{
		Success: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
