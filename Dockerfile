FROM golang:1.13-alpine

RUN apk --no-cache add make bash fish build-base

WORKDIR /project

COPY ./Makefile ./go.mod ./go.sum /project/
RUN make tools

COPY ./ /project

CMD make all
