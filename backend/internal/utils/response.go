package utils

import "encoding/json"

// CitizenResponse, standard API response format
type CitizenResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewCitizenResponse, standard API response
func NewCitizenResponse(success bool, message string, data interface{}) CitizenResponse {
	return CitizenResponse{
		Success: success,
		Message: message,
		Data:    data,
	}
}

// ToJSON, convert CitizenResponse to JSON
func (r CitizenResponse) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

