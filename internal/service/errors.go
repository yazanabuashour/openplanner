package service

type FieldError struct {
	Field   string
	Message string
}

type ValidationError struct {
	Message     string
	FieldErrors []FieldError
}

func (err *ValidationError) Error() string {
	return err.Message
}

type NotFoundError struct {
	Resource string
	ID       string
	Message  string
}

func (err *NotFoundError) Error() string {
	if err.Message != "" {
		return err.Message
	}

	return err.Resource + " not found"
}

type ConflictError struct {
	Message string
}

func (err *ConflictError) Error() string {
	return err.Message
}
