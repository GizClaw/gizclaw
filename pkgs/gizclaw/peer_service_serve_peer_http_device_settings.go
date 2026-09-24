package gizclaw

import "net/http"

func invalidDeviceRequest(message string) *deviceControlError {
	return &deviceControlError{Status: http.StatusBadRequest, Code: publicHTTPInvalidRequestCode, Message: message}
}

func internalDeviceControlError() *deviceControlError {
	return &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
}
