/*
 * Kyma Gateway Metadata API
 *
 * No description provided (generated by Openapi Generator https://github.com/openapitools/openapi-generator)
 *
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package openapi

type Service struct {
	Id string `json:"id,omitempty"`
	Provider string `json:"provider,omitempty"`
	Name string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}