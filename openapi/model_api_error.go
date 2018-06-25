/*
 * Kyma Gateway Metadata API
 *
 * No description provided (generated by Openapi Generator https://github.com/openapitools/openapi-generator)
 *
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

// API Error response body
type ApiError struct {
	// original HTTP error code, should be consistent with the response HTTP code
	Status int32 `json:"status"`
	// classification of the error type, lower case with underscore eg validation_failure
	Type string `json:"type"`
	// descriptive error message for debugging
	Message string `json:"message,omitempty"`
	// link to documentation to investigate further and finding support
	MoreInfo string `json:"moreInfo,omitempty"`
	// list of error causes
	Details []ApiErrorDetail `json:"details,omitempty"`
}
