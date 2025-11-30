package errors_test

import (
	"fmt"

	"backend/pkg/errors"
)

func ExampleNew() {
	err := errors.New("USER_NOT_FOUND", "User with ID 123 not found")
	fmt.Println(err.Error())
	// Output: User with ID 123 not found
}

func ExampleWrap() {
	originalErr := fmt.Errorf("database connection failed")
	err := errors.Wrap(originalErr, "DATABASE_ERROR", "Failed to connect to database")
	fmt.Println(err.Error())
	// Output: Failed to connect to database: database connection failed
}

func ExampleWithDetail() {
	err := errors.New("VALIDATION_ERROR", "Invalid input")
	err = err.WithDetail("field", "email").WithDetail("reason", "invalid format")
	fmt.Println(errors.Format(err))
	// Output: code=VALIDATION_ERROR message="Invalid input" details=[field=email reason=invalid format]
}

func ExampleCommonErrors() {
	// Not found error
	err := errors.NotFound("User")
	fmt.Println(err.Error())

	// Unauthorized error
	err = errors.Unauthorized("Invalid credentials")
	fmt.Println(err.Error())

	// Bad request error
	err = errors.BadRequest("Missing required field: email")
	fmt.Println(err.Error())
	// Output:
	// User not found
	// Invalid credentials
	// Missing required field: email
}

func ExampleErrorChecking() {
	err := errors.NotFound("User")

	// Check error code
	var appErr *errors.Error
	if errors.As(err, &appErr) {
		fmt.Printf("Error code: %s\n", appErr.Code)
		fmt.Printf("Error message: %s\n", appErr.Message)
	}

	// Check if it's a not found error by code
	if appErr != nil && appErr.Code == errors.ErrCodeNotFound {
		fmt.Println("Not found error")
	}
	// Output:
	// Error code: NOT_FOUND
	// Error message: User not found
	// Not found error
}

func ExampleWrapWithDetails() {
	originalErr := fmt.Errorf("connection timeout")
	err := errors.Wrap(originalErr, "DATABASE_ERROR", "Failed to connect")
	err = err.WithDetail("host", "localhost").WithDetail("port", 5432)
	fmt.Println(errors.Format(err))
	// Output: code=DATABASE_ERROR message="Failed to connect" error=connection timeout details=[host=localhost port=5432]
}

func ExampleErrorComparison() {
	err1 := errors.NotFound("User")
	err2 := errors.NotFound("Product")

	// Check if both are NotFound errors by comparing codes
	var appErr1, appErr2 *errors.Error
	if errors.As(err1, &appErr1) && errors.As(err2, &appErr2) {
		if appErr1.Code == appErr2.Code {
			fmt.Println("Both are not found errors")
		}
	}
	// Output: Both are not found errors
}
