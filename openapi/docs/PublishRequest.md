# PublishRequest

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**EventType** | **string** | Type of the event. | 
**EventTypeVersion** | **string** | The version of the event-type. This is applicable to the data payload alone. | 
**EventId** | **string** | Optional publisher provided ID (UUID v4) of the to-be-published event. When omitted, one will be automatically generated. | [optional] 
**EventTime** | [**time.Time**](time.Time.md) | RFC 3339 timestamp of when the event happened. | 
**Data** | [***AnyValue**](AnyValue.md) |  | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


