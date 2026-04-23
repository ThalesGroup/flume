FROM golang:1.25-alpine

RUN apk --no-cache add make bash fish just build-base

WORKDIR /flume

RUN --mount=type=bind,source=./,target=/flume just tools

CMD ["just"]
