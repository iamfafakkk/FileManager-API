package models

import "time"

// StandardResponse is the standard API response wrapper
type StandardResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data"`
	Error     *ErrorInfo  `json:"error"`
	Timestamp time.Time   `json:"timestamp"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code    string `json:"code"`
	Details string `json:"details"`
}

// NewSuccessResponse creates a success response
func NewSuccessResponse(message string, data interface{}) StandardResponse {
	return StandardResponse{
		Success:   true,
		Message:   message,
		Data:      data,
		Error:     nil,
		Timestamp: time.Now(),
	}
}

// NewErrorResponse creates an error response
func NewErrorResponse(message string, code string, details string) StandardResponse {
	return StandardResponse{
		Success: false,
		Message: message,
		Data:    nil,
		Error: &ErrorInfo{
			Code:    code,
			Details: details,
		},
		Timestamp: time.Now(),
	}
}

// PaginatedResponse wraps paginated data
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}
