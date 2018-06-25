# ApiErrorDetail

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Field** | **string** | a bean notation expression specifying the element in request data causing the error, eg product.variants[3].name, this can be empty if violation was not field specific | [optional] 
**Type** | **string** | classification of the error detail type, lower case with underscore eg missing_value, this value must be always interpreted in context of the general error type. | 
**Message** | **string** | descriptive error detail message for debugging | [optional] 
**MoreInfo** | **string** | link to documentation to investigate further and finding support for error detail | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


