# API Endpoints

All error conditions which can happen even to a correctly-written client are
documented. The following are not documented:
- Any error condition which can only happen if a client is buggy
- All endpoints may return 500 on internal server error

# `/challenge`

## Behavior

Generates a new proof of work challenge and stores it in the database.

## Request

Method: `GET`

Request body: None

## Response

Code: 200

Response body:

```json
{
   "work_factor" : 1024,
   "nonce" : "54be07e7445880272d5f36cc56c78b6b"
}
```

# `/report`

## Behavior

- If a challenge solution is provided but not upload key:
  - Allocates new upload token, generates new upload key
  - Stores pending report in database
  - Responds with upload token
- If upload key is provided but not challenge solution:
  - TODO

## New report without upload key

### Request

Method: `POST`

Request body:

```json
{
   "report" : {
      "data" : "9USO+Z30bvZWIKPwZmee0TvkGXBQi7+DqAjtdYZ="
   },
   "challenge" : {
      "solution" : {
         "nonce" : "6e38798e1cf0c5a26fedb35da176a589"
      },
      "challenge" : {
         "nonce" : "54be07e7445880272d5f36cc56c78b6b",
         "work_factor" : 1024
      }
   }
}
```

### Response

Code: 200

Response body:

```json
{
   "upload_key" : "UufO/rTN6adhqkwnNqRUbQ==",
   "upload_token" : "234-226-9"
}
```

## New report with upload key

### Request

Method: `POST`

Request body:

```json
{
   "upload_key" : "UufO/rTN6adhqkwnNqRUbQ==",
   "report" : {
      "data" : "9USO+Z30bvZWIKPwZmee0TvkGXBQi7+DqAjtdYZ="
   }
}
```

### Response

TODO
