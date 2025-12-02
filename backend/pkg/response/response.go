package response

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// Response represents a standard API response
type Response struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Code      string      `json:"code,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	Response
	Pagination Pagination `json:"pagination"`
}

// Pagination holds pagination metadata
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Success sends a success response
func Success(c *fiber.Ctx, data interface{}) error {
	return c.JSON(Response{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// SuccessWithMessage sends a success response with a message
func SuccessWithMessage(c *fiber.Ctx, message string, data interface{}) error {
	return c.JSON(Response{
		Success:   true,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Error sends an error response
func Error(c *fiber.Ctx, statusCode int, message string) error {
	return c.Status(statusCode).JSON(Response{
		Success:   false,
		Error:     message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ErrorWithCode sends an error response with an error code
func ErrorWithCode(c *fiber.Ctx, statusCode int, code, message string) error {
	return c.Status(statusCode).JSON(Response{
		Success:   false,
		Code:      code,
		Error:     message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Paginated sends a paginated response
func Paginated(c *fiber.Ctx, data interface{}, page, limit int, total int64) error {
	totalPages := int((total + int64(limit) - 1) / int64(limit))
	if totalPages == 0 {
		totalPages = 1
	}

	return c.JSON(PaginatedResponse{
		Response: Response{
			Success:   true,
			Data:      data,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Pagination: Pagination{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// BadRequest sends a 400 Bad Request response
func BadRequest(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusBadRequest, message)
}

// Unauthorized sends a 401 Unauthorized response
func Unauthorized(c *fiber.Ctx, message string) error {
	if message == "" {
		message = "Unauthorized"
	}
	return Error(c, fiber.StatusUnauthorized, message)
}

// Forbidden sends a 403 Forbidden response
func Forbidden(c *fiber.Ctx, message string) error {
	if message == "" {
		message = "Forbidden"
	}
	return Error(c, fiber.StatusForbidden, message)
}

// NotFound sends a 404 Not Found response
func NotFound(c *fiber.Ctx, message string) error {
	if message == "" {
		message = "Not Found"
	}
	return Error(c, fiber.StatusNotFound, message)
}

// Conflict sends a 409 Conflict response
func Conflict(c *fiber.Ctx, message string) error {
	if message == "" {
		message = "Conflict"
	}
	return Error(c, fiber.StatusConflict, message)
}

// InternalServerError sends a 500 Internal Server Error response
func InternalServerError(c *fiber.Ctx, message string) error {
	if message == "" {
		message = "Internal Server Error"
	}
	return Error(c, fiber.StatusInternalServerError, message)
}

// Created sends a 201 Created response with message and data
func Created(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(Response{
		Success:   true,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
