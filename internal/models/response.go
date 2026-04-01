package models

// ErrorResponse representa uma resposta de erro
type ErrorResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

// SuccessResponse representa uma resposta de sucesso
type SuccessResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
}

// ValidationError representa um erro de validação
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// ValidationErrorResponse representa uma resposta de erro de validação
type ValidationErrorResponse struct {
	Error   bool              `json:"error"`
	Message string            `json:"message"`
	Details []ValidationError `json:"details"`
}

// NewErrorResponse cria uma nova resposta de erro
func NewErrorResponse(message string) ErrorResponse {
	return ErrorResponse{
		Error:   true,
		Message: message,
	}
}

// NewSuccessResponse cria uma nova resposta de sucesso
func NewSuccessResponse(message string) SuccessResponse {
	return SuccessResponse{
		Error:   false,
		Message: message,
	}
}

// NewValidationErrorResponse cria uma nova resposta de erro de validação
func NewValidationErrorResponse(message string, details []ValidationError) ValidationErrorResponse {
	return ValidationErrorResponse{
		Error:   true,
		Message: message,
		Details: details,
	}
}
