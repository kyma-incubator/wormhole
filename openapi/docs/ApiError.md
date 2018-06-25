# ApiError

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Status** | **int32** | original HTTP error code, should be consistent with the response HTTP code | 
**Type** | **string** | classification of the error type, lower case with underscore eg validation_failure | 
**Message** | **string** | descriptive error message for debugging | [optional] 
**MoreInfo** | **string** | link to documentation to investigate further and finding support | [optional] 
**Details** | [**[]ApiErrorDetail**](APIErrorDetail.md) | list of error causes | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


