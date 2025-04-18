FROM golang:1.19-alpine

RUN apk --no-cache add make bash fish build-base

WORKDIR /flume

COPY ./Makefile ./go.mod ./go.sum /flume/
RUN make tools

COPY ./ /flume

CMD make all
