---
name: vfmp
description: toolkit for interacting with the vfmp project. Supports publishing messages, consuming messages, and gathering information around the current the current state of the application. Use when user asks to interact with a topic such as "get message count for topic" or "publish message to topic" or "peek message on topic" or "consume message from topic".
---
# VFMP

VFMP is a message processing application that allows users to publish and consume messages. It uses a point-to-point architecture meaning a message on a topic is consumed exactly once. New messages are published using the http server and messages are consumed using a tcp client.

To check if VFMP is already running use `curl -s localhost:8080/control/healthcheck`, a non-zero exit code implies vfmp is not running.
If vfmp is not already started, start it in the background using `just start`. You should always ensure vfmp is running before trying anything else, failure to do so will result in unexpected behaviour.

# HTTP server

By default, vfmp starts a http server on port 8080 which means the base url for interacting with the http server will be `localhost:8080`. The endpoints exposed by this server are as follows

## Healthcheck
Check if vfmp is running and healthy

### Command
```bash
curl -s "localhost:8080/control/healthcheck"
```

### Response
A non-zero exit code implies vfmp is not running, a zero exit code means vfmp is ready to accept requests.

## Version
Get the current version of the running vfmp

### Command
```bash
curl -s "localhost:8080/control/version"
```

### Response
Service is available and is running version v1.1.2
Version numbers will follow semver conventions
```
v1.1.2
```

Any other response or non-zero exit code means vfmp is not running

## Create New Message
This endpoint allows you to create a new message for a given topic
### Command
```bash
curl -is -X POST "localhost:8080/messages/$TOPIC" -d "$DATA"
```

### Successful Response
Service successfully created the message
```
HTTP/1.1 200 OK
```

### Error Response
The server failed to create the message
```
HTTP/1.1 500 Internal Server Error
```

Any other response or non-zero exit code means vfmp is not running

## Get Message Count
This endpoint allows you to get the number of messages for a given topic, if the topic does not exist count will always have a value of 0
### Command
```bash
curl -s "localhost:8080/messages/$TOPIC?data=count"
```

### Successful Response
Returns the number of messages as a JSON object with schema `{"count": int}`
```
{"count": 1}
```

### Error Response
The request did not have a valid query parameter
```
HTTP/1.1 400 Bad Request
```

Any other response or non-zero exit code means vfmp is not running

## Peek Message
This endpoint allows you to see what the next message to be consumed will be for a given topic.
### Command
```bash
curl -s "localhost:8080/messages/$TOPIC?data=peek"
```

### Successful Response
returns the next message to be consumed as a JSON object
```
{"topic": "test", "correlationID": "019c520d-1f22-7cdc-bb05-8105e5ae0c01", "data": "Y29udGVudA=="}
```
#### JSON Schema
Here data is given as a base64 encoded string which maps to the bytes given by the data of the message.
```JSON
{
    "topic": string,
    "correlationID": string,
    "data": string
}
```
You can decode this base64 data using the command `echo $DATA | base64 -d -`.
So to get the data of a given message you would run `curl -s localhost:8080/messages/$TOPIC?data=peek | jq -r '.data' | base64 -d -`. NOTE: if the data is not valid UTF-8 then there maybe undefined terminal behaviour

### Error Response
The request did not have a valid query parameter
```
HTTP/1.1 400 Bad Request
```

Any other response or non-zero exit code means vfmp is not running

# Consuming Messages
Unlike peek once a message is consumed it will no longer be available for other clients to read.
To actually consume a message from vfmp you can use the cli application. Always rebuild the cli app before running further commands, you can build it using `just build cli`.
To consume a message from a given topic the server address and topic name must be given to cli app. By default the tcp server will be on port 9090 so it's base address will be `localhost:9090`.

## Flags
The cli binary located at `./bin/cli` has several flags for controlling consumption of messages

- `-address` the address of the vfmp server, defaults to `localhost:9090` (the same as the default tcp server address)
- `-topic` the topic to consume from, defaults to `test`
- `-n` the number of messages to consume
- `-sequence` a comma-separated list of responses to give to the server after receiving a message. Overrides the `-n` flag to be `len(sequence)`

## Consume One Message
This will print to stdout the contents of a message in the format `MSG|$TOPIC|$ID|$DATA`
```bash
./bin/cli -topic=$TOPIC
```

## Consume Multiple Messages
This will print to stdout the contents of three messages in the format `MSG|$TOPIC|$ID|$DATA`
```bash
./bin/cli -topic=$TOPIC -n=3
```

## Custom Responses
Supported response types are:

- `ACK` consume message normally
- `NCK` put message to back of queue
- `DLQ` send message to dead letter queue

```bash
./bin/cli -topic=$TOPIC -sequence=ACK,NCK,DLQ # ACK first message, NCK second message, DLQ final message
```
