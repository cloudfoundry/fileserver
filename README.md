File Server
===========

Bob Loblaw's file server

## Uploading Droplets

Uploading droplets via CC involves crafting a correctly-formed multipart request and polling for the response.  We have an integration for this at the root level.  To run it you must specify (as environment variables):

CC_ADDRESS the hostname for a deployed CC
CC_USERNAME, CC_PASSWORD the basic auth credentials for the droplet upload endpoint
CC_APPGUID a valid app guid on that deployed CC
